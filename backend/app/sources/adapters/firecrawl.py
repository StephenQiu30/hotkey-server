from __future__ import annotations

import hashlib
import json
from collections.abc import Callable
from contextlib import AbstractContextManager
from datetime import UTC, datetime
from typing import Any, Literal, Self
from urllib.parse import urlsplit, urlunsplit

import httpx

from sources.adapters.web_targets import normalize_web_url
from sources.contracts import (
    SourceDocument,
    SourceStopReason,
    WebPageRequest,
    WebPageResult,
)

_VERSION = "firecrawl/2.11.162"
_LOGIN_PATHS = frozenset({"auth", "login", "signin", "sign-in"})
_CHALLENGE_TITLES = ("captcha", "verify you are human", "just a moment")
type Clock = Callable[[], datetime]


class FirecrawlAdapter(AbstractContextManager["FirecrawlAdapter"]):
    """Bounded adapter for the fixed local Firecrawl v2 scrape endpoint."""

    def __init__(
        self,
        *,
        base_url: str,
        enabled: bool,
        allowed_hosts: frozenset[str],
        max_response_bytes: int = 2 * 1024 * 1024,
        transport: httpx.BaseTransport | None = None,
        clock: Clock | None = None,
    ) -> None:
        if not 1 <= max_response_bytes <= 2 * 1024 * 1024:
            raise ValueError("invalid Firecrawl response limit")
        self._base_url = self._validate_base_url(base_url)
        self._enabled = enabled
        self._allowed_hosts = allowed_hosts
        self._max_response_bytes = max_response_bytes
        self._clock = clock or (lambda: datetime.now(UTC))
        self._client = httpx.Client(
            base_url=self._base_url,
            timeout=httpx.Timeout(20),
            transport=transport,
        )

    def __enter__(self) -> Self:
        return self

    def __exit__(self, *args: object) -> None:
        self._client.close()

    @staticmethod
    def _validate_base_url(value: str) -> str:
        try:
            parsed = urlsplit(value)
            port = parsed.port
        except ValueError as error:
            raise ValueError("invalid Firecrawl base URL") from error
        if (
            parsed.scheme not in {"http", "https"}
            or parsed.hostname is None
            or parsed.username is not None
            or parsed.password is not None
            or parsed.query
            or parsed.fragment
            or parsed.path not in {"", "/"}
            or port is None
        ):
            raise ValueError("invalid Firecrawl base URL")
        return urlunsplit((parsed.scheme, parsed.netloc, "", "", ""))

    def fetch_document(self, request: WebPageRequest) -> WebPageResult:
        if not self._enabled:
            return self._failure(SourceStopReason.UNSUPPORTED, calls=0)
        try:
            request_url = normalize_web_url(request.url, allowed_hosts=self._allowed_hosts)
        except ValueError:
            return self._failure(SourceStopReason.ACCESS_DENIED, calls=0)

        payload = {
            "url": request_url,
            "formats": ["markdown"],
            "onlyMainContent": True,
            "maxAge": 0,
            "storeInCache": False,
            "timeout": request.timeout_seconds * 1000,
        }
        try:
            with self._client.stream(
                "POST",
                "/v2/scrape",
                json=payload,
                timeout=request.timeout_seconds,
            ) as response:
                service_reason = self._service_failure(response.status_code)
                if service_reason is not None:
                    return self._failure(service_reason, calls=1)
                body = bytearray()
                for chunk in response.iter_bytes(chunk_size=64 * 1024):
                    body.extend(chunk)
                    if len(body) > self._max_response_bytes:
                        return self._failure(SourceStopReason.BUDGET_EXHAUSTED, calls=1)
        except (httpx.HTTPError, TimeoutError):
            return self._failure(SourceStopReason.UPSTREAM_ERROR, calls=1)

        try:
            decoded = json.loads(body)
            return self._map_response(decoded, request=request, request_url=request_url)
        except (json.JSONDecodeError, TypeError, ValueError, KeyError):
            return self._failure(SourceStopReason.PROTOCOL_ERROR, calls=1)

    def _map_response(
        self, payload: Any, *, request: WebPageRequest, request_url: str
    ) -> WebPageResult:
        if not isinstance(payload, dict) or payload.get("success") is not True:
            return self._failure(SourceStopReason.PROTOCOL_ERROR, calls=1)
        data = payload.get("data")
        if not isinstance(data, dict):
            raise ValueError("Firecrawl data is missing")
        metadata = data.get("metadata")
        markdown = data.get("markdown")
        if not isinstance(metadata, dict) or not isinstance(markdown, str):
            raise ValueError("Firecrawl document is invalid")
        status_code = metadata.get("statusCode")
        if isinstance(status_code, bool) or not isinstance(status_code, int):
            raise ValueError("target status is missing")
        target_reason = self._target_failure(status_code)
        if target_reason is not None:
            return self._failure(target_reason, calls=1, target_status=status_code)
        if not markdown.strip():
            return self._failure(SourceStopReason.SOURCE_EMPTY, calls=1, target_status=status_code)

        raw_final_url = metadata.get("url") or metadata.get("sourceURL")
        if not isinstance(raw_final_url, str):
            raise ValueError("final URL is missing")
        try:
            final_url = normalize_web_url(raw_final_url, allowed_hosts=self._allowed_hosts)
        except ValueError:
            return self._failure(SourceStopReason.ACCESS_DENIED, calls=1, target_status=status_code)
        title = metadata.get("title")
        if title is not None and not isinstance(title, str):
            raise ValueError("document title is invalid")
        if self._looks_like_authentication_challenge(final_url, title):
            return self._failure(
                SourceStopReason.AUTHENTICATION_REQUIRED,
                calls=1,
                target_status=status_code,
            )

        observed_at = self._clock()
        if observed_at.utcoffset() is None:
            raise ValueError("clock must return a timezone-aware datetime")
        published_at = self._published_at(metadata.get("publishedTime"))
        text = markdown[: request.max_content_characters]
        scope: Literal["full", "truncated"] = "full" if len(text) == len(markdown) else "truncated"
        document = SourceDocument(
            request_url=request_url,
            final_url=final_url,
            title=title,
            text=text,
            text_scope=scope,
            observed_at=observed_at.astimezone(UTC),
            published_at=published_at,
            content_fingerprint=hashlib.sha256(text.encode()).hexdigest(),
            extractor_version=_VERSION,
        )
        return WebPageResult(
            document=document,
            stop_reason=None,
            target_status_code=status_code,
            collector_call_count=1,
            target_request_count=None,
        )

    @staticmethod
    def _published_at(value: Any) -> datetime | None:
        if value is None:
            return None
        if not isinstance(value, str):
            raise ValueError("published time is invalid")
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
        if parsed.utcoffset() is None:
            raise ValueError("published time must include a timezone")
        return parsed.astimezone(UTC)

    @staticmethod
    def _looks_like_authentication_challenge(final_url: str, title: str | None) -> bool:
        path_segments = {part.lower() for part in urlsplit(final_url).path.split("/") if part}
        normalized_title = (title or "").strip().lower()
        return bool(path_segments & _LOGIN_PATHS) or any(
            marker in normalized_title for marker in _CHALLENGE_TITLES
        )

    @staticmethod
    def _service_failure(status_code: int) -> SourceStopReason | None:
        if status_code == 429:
            return SourceStopReason.RATE_LIMITED
        if status_code in {401, 403}:
            return SourceStopReason.ACCESS_DENIED
        if status_code >= 500:
            return SourceStopReason.UPSTREAM_ERROR
        if status_code != 200:
            return SourceStopReason.PROTOCOL_ERROR
        return None

    @staticmethod
    def _target_failure(status_code: int) -> SourceStopReason | None:
        if status_code == 401:
            return SourceStopReason.AUTHENTICATION_REQUIRED
        if status_code == 403:
            return SourceStopReason.ACCESS_DENIED
        if status_code == 404:
            return SourceStopReason.NOT_FOUND
        if status_code == 429:
            return SourceStopReason.RATE_LIMITED
        if status_code >= 500:
            return SourceStopReason.UPSTREAM_ERROR
        if not 200 <= status_code < 300:
            return SourceStopReason.PROTOCOL_ERROR
        return None

    @staticmethod
    def _failure(
        reason: SourceStopReason, *, calls: int, target_status: int | None = None
    ) -> WebPageResult:
        return WebPageResult(
            document=None,
            stop_reason=reason,
            target_status_code=target_status,
            collector_call_count=calls,
            target_request_count=None,
        )
