from __future__ import annotations

import calendar
import time
from collections.abc import Callable
from datetime import UTC, datetime
from typing import Any, ClassVar
from urllib.parse import urlsplit

import feedparser
import httpx
from pydantic import ValidationError

from sources.adapters.http_source import (
    HttpSourceAdapter,
    SourceFailureError,
    html_to_text,
    parse_timestamp,
)
from sources.contracts import (
    HotlistEntry,
    HotlistPage,
    SourceCapability,
    SourcePageState,
    SourceStopReason,
)


class RsshubHotlistAdapter(HttpSourceAdapter):
    """Read one allowlisted local RSSHub ranking; feed order is the rank."""

    capabilities: ClassVar[frozenset[SourceCapability]] = frozenset({SourceCapability.HOTLIST})
    adapter_version: ClassVar[str] = "rsshub-hotlist-v1"

    def __init__(
        self,
        *,
        source_key: str,
        feed_url: str,
        allowed_hosts: frozenset[str],
        before_request: Callable[[int], bool],
        cancelled: Callable[[], bool] = lambda: False,
        max_seconds: float = 35,
        transport: httpx.BaseTransport | None = None,
    ) -> None:
        if allowed_hosts != frozenset({"127.0.0.1"}):
            raise ValueError("RSSHub hotlist may access only 127.0.0.1")
        parsed = urlsplit(feed_url)
        if (
            parsed.scheme != "http"
            or parsed.hostname != "127.0.0.1"
            or parsed.port != 1200
            or parsed.username is not None
            or parsed.password is not None
            or parsed.fragment
        ):
            raise ValueError("RSSHub hotlist requires the local RSSHub endpoint")
        super().__init__(
            source_key=source_key,
            allowed_hosts=allowed_hosts,
            before_request=before_request,
            cancelled=cancelled,
            max_requests=1,
            max_seconds=max_seconds,
            transport=transport,
        )
        self._feed_url = feed_url

    def fetch_hotlist(self) -> HotlistPage:
        if not self._lock.acquire(blocking=False):
            raise ValueError("a hotlist adapter cannot be shared between concurrent runs")
        try:
            if self._deadline is not None:
                raise ValueError("a hotlist adapter can fetch only once")
            self._deadline = time.monotonic() + self._max_seconds
            try:
                parsed = feedparser.parse(self._get_bytes(self._feed_url))
                if parsed.get("bozo") and not parsed.get("entries"):
                    raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
                if len(parsed.entries) > 100:
                    raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
                items = tuple(
                    self._entry(raw, rank) for rank, raw in enumerate(parsed.entries[:100], 1)
                )
                state = SourcePageState.COMPLETE if items else SourcePageState.EMPTY
                reason = SourceStopReason.END_OF_RESULTS if items else SourceStopReason.SOURCE_EMPTY
            except SourceFailureError as error:
                items, state, reason = (), SourcePageState.STOPPED, error.reason
            except (httpx.HTTPError, TimeoutError):
                items, state, reason = (), SourcePageState.STOPPED, SourceStopReason.UPSTREAM_ERROR
            except (ValueError, UnicodeError, ValidationError, TypeError, OverflowError):
                items, state, reason = (), SourcePageState.STOPPED, SourceStopReason.PROTOCOL_ERROR
            return HotlistPage(
                source_key=self.source_key,
                state=state,
                items=items,
                stop_reason=reason,
                observed_at=datetime.now(UTC),
                request_count=self._request_count,
                adapter_version=self.adapter_version,
            )
        finally:
            self._lock.release()

    @staticmethod
    def _entry(raw: Any, rank: int) -> HotlistEntry:
        title = html_to_text(raw.get("title"))
        url = raw.get("link")
        if not title or not isinstance(url, str):
            raise ValueError("RSSHub hotlist entry lacks title or URL")
        summary = raw.get("summary")
        if not summary and raw.get("content"):
            summary = raw["content"][0].get("value")
        published = raw.get("published_parsed") or raw.get("updated_parsed")
        published_at = (
            parse_timestamp(calendar.timegm(published))
            if published is not None
            else parse_timestamp(raw.get("published") or raw.get("updated"))
        )
        heat = raw.get("heat")
        return HotlistEntry(
            rank=rank,
            title=title[:2000],
            url=url,
            summary=(html_to_text(summary) or "")[:100_000] or None,
            published_at=published_at,
            heat=str(heat)[:256] if heat is not None else None,
        )
