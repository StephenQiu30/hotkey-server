from __future__ import annotations

import hashlib
import json
from collections.abc import Callable
from contextlib import suppress
from datetime import UTC, datetime
from typing import ClassVar
from urllib.parse import urlsplit

import httpx

from sources.adapters.http_source import (
    HttpSourceAdapter,
    SourceFailureError,
    html_to_text,
    parse_timestamp,
)
from sources.contracts import (
    SearchRequest,
    SocialSourceCapability,
    SourceCapability,
    SourcePage,
    SourcePost,
    SourceRequest,
    SourceStopReason,
)

_MAX_PAGES = 5


def _published_at(value: object) -> datetime | None:
    """SearXNG runs with TZ=UTC and emits naive ISO timestamps, so naive means UTC."""
    if isinstance(value, str):
        with suppress(ValueError):
            parsed = datetime.fromisoformat(value)
            if parsed.tzinfo is None:
                return parsed.replace(tzinfo=UTC)
    return parse_timestamp(value)


class WebSearchAdapter(HttpSourceAdapter):
    """Keyword news search through the self-hosted SearXNG JSON API (DEC-001-107).

    Results are identified by a hash of their URL, so the same article found by different
    engines is stored once. Results without a publish date are returned; the keyword
    discovery commit drops them when a time window is required. As measured on 2026-09-25,
    only the "duckduckgo news" engine returns publish dates, so it is the default engine.
    """

    capabilities: ClassVar[frozenset[SocialSourceCapability]] = frozenset({SourceCapability.SEARCH})
    adapter_version: ClassVar[str] = "searxng-json/news"

    def __init__(
        self,
        *,
        base_url: str,
        allowed_hosts: frozenset[str],
        before_request: Callable[[int], bool],
        cancelled: Callable[[], bool] = lambda: False,
        max_requests: int = _MAX_PAGES,
        max_seconds: float = 60,
        categories: str = "news",
        engines: str = "duckduckgo news",
        source_key: str = "web",
        transport: httpx.BaseTransport | None = None,
    ) -> None:
        parsed = urlsplit(base_url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError("base_url must be an http(s) URL")
        super().__init__(
            source_key=source_key,
            allowed_hosts=allowed_hosts,
            before_request=before_request,
            cancelled=cancelled,
            max_requests=max_requests,
            max_seconds=max_seconds,
            transport=transport,
        )
        if parsed.hostname not in self._allowed_hosts:
            raise ValueError("base_url host must be allowlisted")
        self._search_url = base_url.rstrip("/") + "/search"
        self._categories = categories
        self._engines = engines

    def _fetch(self, request: SourceRequest) -> SourcePage:
        if not isinstance(request, SearchRequest):
            raise SourceFailureError(SourceStopReason.UNSUPPORTED)
        page_number = 1
        if request.page_token is not None:
            if not request.page_token.isdigit():
                raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
            page_number = int(request.page_token)
        params = {"q": request.query, "format": "json", "pageno": str(page_number)}
        # SearXNG adds category engines to explicit engines, so send only one of them.
        if self._engines:
            params["engines"] = self._engines
        else:
            params["categories"] = self._categories
        payload = json.loads(self._get_bytes(self._search_url, params=params))
        results = payload.get("results") if isinstance(payload, dict) else None
        if not isinstance(results, list):
            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        items: list[SourcePost] = []
        seen: set[str] = set()
        for result in results:
            post = self._post(result)
            if post is None or post.external_id in seen:
                continue
            seen.add(post.external_id)
            items.append(post)
            if len(items) >= request.page_size:
                break
        has_more = bool(results) and page_number < _MAX_PAGES
        return self._page(request, tuple(items), str(page_number + 1) if has_more else None)

    def _post(self, result: object) -> SourcePost | None:
        if not isinstance(result, dict):
            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        url = result.get("url")
        if not isinstance(url, str) or not url.startswith(("http://", "https://")):
            return None
        if len(url) > 2048:
            return None
        title = result.get("title") if isinstance(result.get("title"), str) else None
        text = html_to_text(result.get("content"))
        return SourcePost(
            source_key=self.source_key,
            external_id="url:" + hashlib.sha256(url.encode()).hexdigest(),
            author_external_id=None,
            published_at=_published_at(result.get("publishedDate")),
            title=title[:2000] if title else None,
            text=text,
            text_scope="truncated" if text else None,
            like_count=None,
            comment_count=None,
            repost_count=None,
            canonical_url=url,
        )
