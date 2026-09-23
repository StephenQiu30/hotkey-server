from __future__ import annotations

import json
import threading
import time
from collections.abc import Callable
from contextlib import suppress
from datetime import UTC, datetime, timedelta
from typing import Any

import httpx
from pydantic import SecretStr, ValidationError

from sources.contracts import (
    SearchRequest,
    SourceCapability,
    SourcePage,
    SourcePageState,
    SourcePost,
    SourceRequest,
    SourceSort,
    SourceStopReason,
)

_SEARCH_URL = "https://api.x.com/2/tweets/search/recent"
_POST_FIELDS = "id,text,created_at,lang,conversation_id,public_metrics"
_MAX_RESPONSE_BYTES = 4 * 1024 * 1024


class _SourceFailureError(Exception):
    def __init__(self, reason: SourceStopReason, retry_at: datetime | None = None) -> None:
        super().__init__(reason.value)
        self.reason = reason
        self.retry_at = retry_at


class XApiAdapter:
    """Offline search; authorize max posts before each call, then settle count or unknown."""

    source_key = "x"
    capabilities = frozenset({SourceCapability.SEARCH})

    def __init__(
        self,
        *,
        token: SecretStr,
        transport: httpx.MockTransport,
        authorize_request: Callable[[int, int], bool],
        settle_request: Callable[[int, int | None], None],
        max_requests: int = 20,
        max_seconds: float = 90,
        cancelled: Callable[[], bool] = lambda: False,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        raw_token = token.get_secret_value()
        if not raw_token or any(ord(char) < 33 or ord(char) > 126 for char in raw_token):
            raise ValueError("invalid developer token")
        if not isinstance(transport, httpx.MockTransport):
            raise ValueError("the offline adapter requires a mock transport")
        if max_requests < 1 or not 0 < max_seconds <= 90:
            raise ValueError("invalid source request limits")
        self._token = token
        self._transport = transport
        self._authorize_request = authorize_request
        self._settle_request = settle_request
        self._max_requests = max_requests
        self._max_seconds = max_seconds
        self._cancelled = cancelled
        self._clock = clock or (lambda: datetime.now(UTC))
        self._deadline: float | None = None
        self._request_count = 0
        self._request_scope: dict[str, Any] | None = None
        self._seen_tokens: set[str] = set()
        self._terminal: tuple[SourceStopReason, datetime | None] | None = None
        self._completed: SourcePage | None = None
        self._lock = threading.Lock()

    def fetch_page(self, request: SourceRequest) -> SourcePage:
        if not self._lock.acquire(blocking=False):
            raise ValueError("a source adapter cannot be shared between concurrent runs")
        start_count = self._request_count
        try:
            if self._deadline is None:
                self._deadline = time.monotonic() + self._max_seconds
            scope = request.model_dump(exclude={"page_token", "page_size"})
            if self._request_scope is not None and scope != self._request_scope:
                raise ValueError("a source adapter belongs to one collection scope")
            self._request_scope = scope
            try:
                if self._completed is not None:
                    return self._completed.model_copy(update={"request_count": 0})
                if self._terminal is not None:
                    raise _SourceFailureError(*self._terminal)
                if not isinstance(request, SearchRequest) or request.source_key != "x":
                    raise _SourceFailureError(SourceStopReason.UNSUPPORTED)
                if request.watermark is not None or request.page_size < 10:
                    raise _SourceFailureError(SourceStopReason.UNSUPPORTED)
                if (request.starts_at is None) != (request.ends_at is None):
                    raise _SourceFailureError(SourceStopReason.UNSUPPORTED)
                if request.starts_at is not None and request.ends_at is not None:
                    if (
                        request.starts_at.utcoffset() != timedelta(0)
                        or request.ends_at.utcoffset() != timedelta(0)
                        or request.starts_at >= request.ends_at
                    ):
                        raise _SourceFailureError(SourceStopReason.UNSUPPORTED)
                    now = self._clock()
                    if now.utcoffset() != timedelta(0):
                        raise ValueError("source clock must be UTC")
                    if request.starts_at < now - timedelta(days=7) or request.ends_at > now:
                        raise _SourceFailureError(SourceStopReason.UNSUPPORTED)
                if request.page_token is not None:
                    if request.page_token in self._seen_tokens:
                        raise _SourceFailureError(SourceStopReason.CURSOR_LOOP)
                    self._seen_tokens.add(request.page_token)
                page = self._fetch_search(request)
            except _SourceFailureError as error:
                page = self._stopped(request, error.reason, error.retry_at)
            except (httpx.HTTPError, TimeoutError):
                page = self._stopped(request, SourceStopReason.UPSTREAM_ERROR)
            except (json.JSONDecodeError, UnicodeError, ValidationError):
                page = self._stopped(request, SourceStopReason.PROTOCOL_ERROR)
            if page.state in {SourcePageState.STOPPED, SourcePageState.PARTIAL}:
                assert page.stop_reason is not None
                self._terminal = (page.stop_reason, page.retry_at)
            elif page.state in {SourcePageState.COMPLETE, SourcePageState.EMPTY}:
                self._completed = page
            return page.model_copy(update={"request_count": self._request_count - start_count})
        finally:
            self._lock.release()

    def _fetch_search(self, request: SearchRequest) -> SourcePage:
        if self._cancelled():
            raise _SourceFailureError(SourceStopReason.CANCELLED)
        assert self._deadline is not None
        remaining = self._deadline - time.monotonic()
        if remaining <= 0 or self._request_count >= self._max_requests:
            raise _SourceFailureError(SourceStopReason.BUDGET_EXHAUSTED)
        next_attempt = self._request_count + 1
        if not self._authorize_request(next_attempt, request.page_size):
            raise _SourceFailureError(SourceStopReason.BUDGET_EXHAUSTED)
        self._request_count = next_attempt
        billable_posts: int | None = None
        try:
            params = {
                "query": request.query,
                "sort_order": "recency" if request.sort is SourceSort.LATEST else "relevancy",
                "max_results": str(request.page_size),
                "post.fields": _POST_FIELDS,
            }
            if request.page_token is not None:
                params["next_token"] = request.page_token
            if request.starts_at is not None and request.ends_at is not None:
                params["start_time"] = request.starts_at.isoformat().replace("+00:00", "Z")
                params["end_time"] = request.ends_at.isoformat().replace("+00:00", "Z")
            with (
                httpx.Client(
                    transport=self._transport,
                    follow_redirects=False,
                    timeout=min(10.0, remaining),
                ) as client,
                client.stream(
                    "GET",
                    _SEARCH_URL,
                    params=params,
                    headers={"Authorization": f"Bearer {self._token.get_secret_value()}"},
                ) as response,
            ):
                self._classify_response(response)
                content = bytearray()
                for chunk in response.iter_bytes():
                    if self._cancelled():
                        raise _SourceFailureError(SourceStopReason.CANCELLED)
                    content.extend(chunk)
                    if len(content) > _MAX_RESPONSE_BYTES:
                        raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
                    if time.monotonic() >= self._deadline:
                        raise _SourceFailureError(SourceStopReason.BUDGET_EXHAUSTED)
                if time.monotonic() >= self._deadline:
                    raise _SourceFailureError(SourceStopReason.BUDGET_EXHAUSTED)
            page = self._parse_page(request, json.loads(content))
            billable_posts = len(page.items)
            return page
        finally:
            self._settle_request(next_attempt, billable_posts)

    @staticmethod
    def _classify_response(response: httpx.Response) -> None:
        reason = {
            401: SourceStopReason.AUTHENTICATION_REQUIRED,
            403: SourceStopReason.ACCESS_DENIED,
            429: SourceStopReason.RATE_LIMITED,
        }.get(response.status_code)
        if reason is not None:
            retry_at = None
            if reason is SourceStopReason.RATE_LIMITED:
                with suppress(KeyError, ValueError, OverflowError, OSError):
                    retry_at = datetime.fromtimestamp(
                        int(response.headers["x-rate-limit-reset"]), UTC
                    )
            raise _SourceFailureError(reason, retry_at)
        if response.status_code >= 500:
            raise _SourceFailureError(SourceStopReason.UPSTREAM_ERROR)
        if response.status_code != 200:
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)

    def _parse_page(self, request: SearchRequest, payload: object) -> SourcePage:
        if not isinstance(payload, dict) or payload.get("errors") or "includes" in payload:
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        meta = payload.get("meta")
        if not isinstance(meta, dict) or type(meta.get("result_count")) is not int:
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        raw_items = payload.get("data", [])
        if not isinstance(raw_items, list) or len(raw_items) != meta["result_count"]:
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        if len(raw_items) > request.page_size:
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        next_token = meta.get("next_token")
        if next_token is not None and (not isinstance(next_token, str) or not next_token):
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        posts = tuple(self._parse_post(item) for item in raw_items)
        if next_token is not None and next_token == request.page_token:
            raise _SourceFailureError(SourceStopReason.CURSOR_LOOP)
        state = (
            SourcePageState.MORE
            if next_token is not None
            else SourcePageState.COMPLETE
            if posts
            else SourcePageState.EMPTY
        )
        return SourcePage(
            source_key="x",
            capability=SourceCapability.SEARCH,
            state=state,
            items=posts,
            next_page_token=next_token,
            watermark=None,
            stop_reason=(
                None
                if next_token is not None
                else SourceStopReason.END_OF_RESULTS
                if posts
                else SourceStopReason.SOURCE_EMPTY
            ),
            observed_at=datetime.now(UTC),
            adapter_version="x-api-v2/recent-search",
        )

    @staticmethod
    def _parse_post(value: object) -> SourcePost:
        if not isinstance(value, dict):
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        identifier = value.get("id")
        author_id = value.get("author_id")
        if not isinstance(identifier, str) or not identifier.isascii() or not identifier.isdigit():
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        if not isinstance(author_id, str) or not author_id.isascii() or not author_id.isdigit():
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        metrics = value.get("public_metrics")
        if metrics is not None and not isinstance(metrics, dict):
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        metrics = metrics or {}
        referenced = value.get("referenced_posts") or []
        if not isinstance(referenced, list):
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        targets: dict[str, str] = {}
        for reference in referenced:
            if not isinstance(reference, dict):
                raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
            kind, target = reference.get("type"), reference.get("id")
            if kind in {"replied_to", "quoted", "retweeted"} and isinstance(target, str):
                targets[kind] = target
        published = value.get("created_at")
        try:
            published_at = datetime.fromisoformat(published) if isinstance(published, str) else None
        except ValueError as error:
            raise _SourceFailureError(SourceStopReason.PROTOCOL_ERROR) from error
        return SourcePost(
            source_key="x",
            external_id=identifier,
            author_external_id=author_id,
            published_at=published_at,
            text=value.get("text"),
            language=value.get("lang"),
            like_count=metrics.get("like_count"),
            comment_count=metrics.get("reply_count"),
            repost_count=metrics.get("repost_count"),
            canonical_url=f"https://x.com/i/web/status/{identifier}",
            conversation_external_id=value.get("conversation_id"),
            parent_external_id=targets.get("replied_to"),
            quote_external_id=targets.get("quoted"),
            repost_external_id=targets.get("retweeted"),
            text_scope=None,
        )

    @staticmethod
    def _stopped(
        request: SourceRequest,
        reason: SourceStopReason,
        retry_at: datetime | None = None,
    ) -> SourcePage:
        return SourcePage(
            source_key="x",
            capability=request.capability,
            state=SourcePageState.STOPPED,
            items=(),
            next_page_token=None,
            watermark=None,
            stop_reason=reason,
            observed_at=datetime.now(UTC),
            adapter_version="x-api-v2/recent-search",
            retry_at=retry_at,
        )
