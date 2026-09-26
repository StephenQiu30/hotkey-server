from __future__ import annotations

import calendar
import hashlib
from collections.abc import Callable
from typing import Any, ClassVar
from urllib.parse import quote, urlsplit

import feedparser
import httpx

from sources.adapters.http_source import (
    HttpSourceAdapter,
    SourceFailureError,
    html_to_text,
    parse_timestamp,
)
from sources.contracts import (
    SocialSourceCapability,
    SourceCapability,
    SourcePage,
    SourcePost,
    SourceRequest,
    SourceStopReason,
)

_QUERY_PLACEHOLDER = "{query}"
_MAX_EXTERNAL_ID = 512


def _external_id(entry: Any) -> str | None:
    raw = entry.get("id") or entry.get("link")
    if not isinstance(raw, str):
        return None
    raw = raw.strip()
    if not raw:
        return None
    if len(raw) > _MAX_EXTERNAL_ID or any(ord(char) < 32 or ord(char) == 127 for char in raw):
        return "sha256:" + hashlib.sha256(raw.encode()).hexdigest()
    return raw


def _published_at(entry: Any) -> object:
    for key in ("published_parsed", "updated_parsed"):
        parsed = entry.get(key)
        if parsed is not None:
            return calendar.timegm(parsed)
    return entry.get("published") or entry.get("updated")


class RssSourceAdapter(HttpSourceAdapter):
    """Keyword search over one RSS/Atom feed template (RSSHub, Google News, Reddit, hnrss).

    A template containing `{query}` receives the URL-encoded query; a template without it
    (for example a hot list) is fetched as-is and filtered later by the topic rules.
    Feeds have no pagination, so every search returns a single complete page.
    """

    capabilities: ClassVar[frozenset[SocialSourceCapability]] = frozenset({SourceCapability.SEARCH})
    adapter_version: ClassVar[str] = "feedparser-6"

    def __init__(
        self,
        *,
        source_key: str,
        feed_url_template: str,
        allowed_hosts: frozenset[str],
        before_request: Callable[[int], bool],
        cancelled: Callable[[], bool] = lambda: False,
        max_requests: int = 1,
        max_seconds: float = 60,
        transport: httpx.BaseTransport | None = None,
    ) -> None:
        parsed = urlsplit(feed_url_template)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError("feed_url_template must be an http(s) URL")
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
            raise ValueError("feed_url_template host must be allowlisted")
        self._template = feed_url_template

    def _fetch(self, request: SourceRequest) -> SourcePage:
        if request.capability is not SourceCapability.SEARCH or request.page_token is not None:
            raise SourceFailureError(SourceStopReason.UNSUPPORTED)
        assert request.capability is SourceCapability.SEARCH
        url = self._template.replace(_QUERY_PLACEHOLDER, quote(request.query, safe=""))
        feed = feedparser.parse(self._get_bytes(url))
        if feed.get("bozo") and not feed.get("entries"):
            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        items: list[SourcePost] = []
        seen: set[str] = set()
        for entry in feed.get("entries", []):
            external_id = _external_id(entry)
            if external_id is None or external_id in seen:
                continue
            seen.add(external_id)
            title = html_to_text(entry.get("title"))
            summary = entry.get("summary")
            if not summary and entry.get("content"):
                summary = entry["content"][0].get("value")
            text = html_to_text(summary)
            link = entry.get("link")
            author = entry.get("author")
            items.append(
                SourcePost(
                    source_key=self.source_key,
                    external_id=external_id,
                    author_external_id=None,
                    author_name=author[:256] if isinstance(author, str) and author else None,
                    published_at=parse_timestamp(_published_at(entry)),
                    title=title[:2000] if title else None,
                    text=text[:100_000] if text else None,
                    text_scope="truncated" if text else None,
                    like_count=None,
                    comment_count=None,
                    repost_count=None,
                    canonical_url=(
                        link
                        if isinstance(link, str)
                        and link.startswith(("http://", "https://"))
                        and len(link) <= 2048
                        else None
                    ),
                )
            )
            if len(items) >= request.page_size:
                break
        return self._page(request, tuple(items), None)
