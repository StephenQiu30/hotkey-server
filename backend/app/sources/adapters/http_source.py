from __future__ import annotations

import threading
import time
from collections.abc import Callable, Mapping
from datetime import UTC, datetime
from email.utils import parsedate_to_datetime
from typing import ClassVar
from urllib.parse import urljoin, urlsplit

import httpx
from bs4 import BeautifulSoup
from pydantic import ValidationError

from sources.adapters.web_targets import normalize_web_host
from sources.contracts import (
    SourceCapability,
    SourceComment,
    SourcePage,
    SourcePageState,
    SourcePost,
    SourceRequest,
    SourceStopReason,
)

_MAX_RESPONSE_BYTES = 4 * 1024 * 1024
_MAX_REDIRECTS = 5
_LOCAL_SERVICE_HOSTS = frozenset({"127.0.0.1", "localhost"})
_USER_AGENT = "HotKey/0.1 (+self-hosted research monitor)"


class SourceFailureError(Exception):
    def __init__(self, reason: SourceStopReason, retry_at: datetime | None = None) -> None:
        super().__init__(reason.value)
        self.reason = reason
        self.retry_at = retry_at


def html_to_text(value: str | None) -> str | None:
    """Collapse an HTML fragment into plain text; empty results become None."""
    if not value:
        return None
    text = BeautifulSoup(value, "html.parser").get_text(" ", strip=True)
    return " ".join(text.split()) or None


def parse_timestamp(value: object) -> datetime | None:
    """Parse ISO-8601, RFC 2822 or epoch seconds into an aware UTC datetime."""
    if isinstance(value, bool):
        return None
    if isinstance(value, int | float):
        return datetime.fromtimestamp(value, UTC)
    if not isinstance(value, str) or not value.strip():
        return None
    raw = value.strip()
    try:
        parsed = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except ValueError:
        try:
            parsed = parsedate_to_datetime(raw)
        except (TypeError, ValueError):
            return None
    if parsed.tzinfo is None:
        return None
    return parsed.astimezone(UTC)


class HttpSourceAdapter:
    """Shared request budget, deadline, cancellation and paging rules for HTTP sources.

    Subclasses implement `_fetch` and call `_get_bytes` for each upstream request.
    The executor settles usage per page through `SourcePage.request_count`.
    """

    capabilities: ClassVar[frozenset[SourceCapability]]
    adapter_version: ClassVar[str]

    def __init__(
        self,
        *,
        source_key: str,
        allowed_hosts: frozenset[str],
        before_request: Callable[[int], bool],
        cancelled: Callable[[], bool] = lambda: False,
        max_requests: int = 10,
        max_seconds: float = 60,
        transport: httpx.BaseTransport | None = None,
        request_timeout: float = 15.0,
    ) -> None:
        if max_requests < 1 or not 0 < max_seconds <= 300 or not 0 < request_timeout <= 60:
            raise ValueError("invalid source request limits")
        self._allowed_hosts = self._validate_allowed_hosts(allowed_hosts)
        self._source_key = source_key
        self._before_request = before_request
        self._cancelled = cancelled
        self._max_requests = max_requests
        self._max_seconds = max_seconds
        self._transport = transport
        self._request_timeout = request_timeout
        self._deadline: float | None = None
        self._request_count = 0
        self._request_scope: dict[str, object] | None = None
        self._seen_tokens: set[str] = set()
        self._terminal: tuple[SourceStopReason, datetime | None] | None = None
        self._completed: SourcePage | None = None
        self._lock = threading.Lock()

    @property
    def source_key(self) -> str:
        return self._source_key

    def fetch_page(self, request: SourceRequest) -> SourcePage:
        if not self._lock.acquire(blocking=False):
            raise ValueError("a source adapter cannot be shared between concurrent runs")
        start_count = self._request_count
        try:
            if self._deadline is None:
                self._deadline = time.monotonic() + self._max_seconds
            scope = request.model_dump(exclude={"page_token", "page_size"}, warnings=False)
            if self._request_scope is not None and scope != self._request_scope:
                raise ValueError("a source adapter belongs to one collection scope")
            self._request_scope = scope
            try:
                if self._completed is not None:
                    return self._completed.model_copy(update={"request_count": 0})
                if self._terminal is not None:
                    raise SourceFailureError(*self._terminal)
                if (
                    request.source_key != self._source_key
                    or request.capability not in self.capabilities
                    or request.watermark is not None
                ):
                    raise SourceFailureError(SourceStopReason.UNSUPPORTED)
                if request.page_token is not None:
                    if request.page_token in self._seen_tokens:
                        raise SourceFailureError(SourceStopReason.CURSOR_LOOP)
                    self._seen_tokens.add(request.page_token)
                page = self._fetch(request)
            except SourceFailureError as error:
                page = self._stopped(request, error.reason, error.retry_at)
            except (httpx.HTTPError, TimeoutError):
                page = self._stopped(request, SourceStopReason.UPSTREAM_ERROR)
            except (ValueError, UnicodeError, ValidationError):
                page = self._stopped(request, SourceStopReason.PROTOCOL_ERROR)
            if page.state in {SourcePageState.STOPPED, SourcePageState.PARTIAL}:
                assert page.stop_reason is not None
                self._terminal = (page.stop_reason, page.retry_at)
            elif page.state in {SourcePageState.COMPLETE, SourcePageState.EMPTY}:
                self._completed = page
            return page.model_copy(update={"request_count": self._request_count - start_count})
        finally:
            self._lock.release()

    def _fetch(self, request: SourceRequest) -> SourcePage:
        raise NotImplementedError

    def _get_bytes(
        self,
        url: str,
        *,
        params: Mapping[str, str] | None = None,
        headers: Mapping[str, str] | None = None,
    ) -> bytes:
        assert self._deadline is not None
        request_headers = {"User-Agent": _USER_AGENT, **(headers or {})}
        next_url = url
        next_params = params
        redirect_count = 0
        self._require_allowed_url(next_url)
        with httpx.Client(
            transport=self._transport,
            follow_redirects=False,
            timeout=self._request_timeout,
        ) as client:
            while True:
                remaining = self._authorize_request()
                with client.stream(
                    "GET",
                    next_url,
                    params=next_params,
                    headers=request_headers,
                    timeout=min(self._request_timeout, remaining),
                ) as response:
                    if response.is_redirect:
                        location = response.headers.get("location")
                        if location is None or redirect_count >= _MAX_REDIRECTS:
                            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
                        next_url = urljoin(str(response.url), location)
                        self._require_allowed_url(next_url)
                        next_params = None
                        redirect_count += 1
                        continue
                    self._classify(response)
                    content = bytearray()
                    for chunk in response.iter_bytes():
                        if self._cancelled():
                            raise SourceFailureError(SourceStopReason.CANCELLED)
                        content.extend(chunk)
                        if len(content) > _MAX_RESPONSE_BYTES:
                            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
                        if time.monotonic() >= self._deadline:
                            raise SourceFailureError(SourceStopReason.BUDGET_EXHAUSTED)
                    return bytes(content)

    def _authorize_request(self) -> float:
        if self._cancelled():
            raise SourceFailureError(SourceStopReason.CANCELLED)
        assert self._deadline is not None
        remaining = self._deadline - time.monotonic()
        if remaining <= 0 or self._request_count >= self._max_requests:
            raise SourceFailureError(SourceStopReason.BUDGET_EXHAUSTED)
        attempt = self._request_count + 1
        if not self._before_request(attempt):
            raise SourceFailureError(SourceStopReason.BUDGET_EXHAUSTED)
        self._request_count = attempt
        return remaining

    @staticmethod
    def _validate_allowed_hosts(allowed_hosts: frozenset[str]) -> frozenset[str]:
        if not allowed_hosts:
            raise ValueError("allowed_hosts must not be empty")
        for host in allowed_hosts:
            if (
                not host
                or host != host.strip()
                or host != host.lower()
                or any(ord(character) < 32 or ord(character) == 127 for character in host)
            ):
                raise ValueError("allowed_hosts must contain lowercase hostnames")
            try:
                parsed = urlsplit(f"//{host}")
                port = parsed.port
            except ValueError as error:
                raise ValueError("allowed_hosts contains an invalid hostname") from error
            if (
                parsed.hostname != host
                or parsed.username is not None
                or parsed.password is not None
                or port is not None
                or parsed.path
                or parsed.query
                or parsed.fragment
            ):
                raise ValueError("allowed_hosts contains an invalid hostname")
        return allowed_hosts

    def _require_allowed_url(self, url: str) -> None:
        try:
            parsed = urlsplit(url)
            _ = parsed.port
        except ValueError as error:
            raise SourceFailureError(SourceStopReason.ACCESS_DENIED) from error
        if (
            url != url.strip()
            or any(ord(character) < 32 or ord(character) == 127 for character in url)
            or parsed.scheme.lower() not in {"http", "https"}
            or parsed.hostname is None
            or parsed.username is not None
            or parsed.password is not None
            or parsed.hostname not in self._allowed_hosts
        ):
            raise SourceFailureError(SourceStopReason.ACCESS_DENIED)
        if parsed.hostname in _LOCAL_SERVICE_HOSTS:
            return
        try:
            normalize_web_host(parsed.hostname)
        except ValueError as error:
            raise SourceFailureError(SourceStopReason.ACCESS_DENIED) from error

    @staticmethod
    def _classify(response: httpx.Response) -> None:
        status = response.status_code
        if status == 429:
            retry_at = None
            retry_after = response.headers.get("retry-after")
            if retry_after is not None and retry_after.isdigit():
                retry_at = datetime.fromtimestamp(time.time() + int(retry_after), UTC)
            raise SourceFailureError(SourceStopReason.RATE_LIMITED, retry_at)
        if status == 401:
            raise SourceFailureError(SourceStopReason.AUTHENTICATION_REQUIRED)
        if status == 403:
            raise SourceFailureError(SourceStopReason.ACCESS_DENIED)
        if status == 404:
            raise SourceFailureError(SourceStopReason.NOT_FOUND)
        if status >= 500:
            raise SourceFailureError(SourceStopReason.UPSTREAM_ERROR)
        if status != 200:
            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)

    def _page(
        self,
        request: SourceRequest,
        items: tuple[SourcePost, ...] | tuple[SourceComment, ...],
        next_token: str | None,
    ) -> SourcePage:
        if next_token is not None and next_token == request.page_token:
            raise SourceFailureError(SourceStopReason.CURSOR_LOOP)
        if next_token is not None:
            state, reason = SourcePageState.MORE, None
        elif items:
            state, reason = SourcePageState.COMPLETE, SourceStopReason.END_OF_RESULTS
        else:
            state, reason = SourcePageState.EMPTY, SourceStopReason.SOURCE_EMPTY
        return SourcePage(
            source_key=self._source_key,
            capability=request.capability,
            state=state,
            items=items,
            next_page_token=next_token,
            watermark=None,
            stop_reason=reason,
            observed_at=datetime.now(UTC),
            adapter_version=self.adapter_version,
        )

    def _stopped(
        self,
        request: SourceRequest,
        reason: SourceStopReason,
        retry_at: datetime | None = None,
    ) -> SourcePage:
        return SourcePage(
            source_key=request.source_key,
            capability=request.capability,
            state=SourcePageState.STOPPED,
            items=(),
            next_page_token=None,
            watermark=None,
            stop_reason=reason,
            observed_at=datetime.now(UTC),
            adapter_version=self.adapter_version,
            retry_at=retry_at,
        )
