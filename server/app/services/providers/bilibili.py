from __future__ import annotations

import asyncio
from datetime import datetime, timezone

import httpx

from server.app.core.settings import settings

from .base import BaseProvider, Candidate, SourceIngestionError
from .registry import register_provider
from .utils import strip_html


BILIBILI_SEARCH_URL = "https://api.bilibili.com/x/web-interface/search/type"


@register_provider("bilibili", "bili")
class BilibiliProvider(BaseProvider):
    source_type = "bilibili"

    async def fetch_hot_topics(self, query: str | None = None) -> list[Candidate]:
        return await asyncio.to_thread(_fetch_bilibili, self.source.config, self.source.id, self.keyword.id, query or self.keyword.keyword)


def _fetch_bilibili(source_config: dict[str, object], source_id: int, keyword_id: int | None, query: str) -> list[Candidate]:
    limit = int(source_config.get("limit") or settings.source_fetch_limit)
    params = {"search_type": "video", "keyword": query, "page": 1}
    headers = {"User-Agent": "Mozilla/5.0"}
    try:
        response = httpx.get(BILIBILI_SEARCH_URL, headers=headers, params=params, timeout=20)
        response.raise_for_status()
        payload = response.json()
    except Exception as exc:  # noqa: BLE001
        raise SourceIngestionError(f"Bilibili fetch failed: {exc}") from exc

    candidates: list[Candidate] = []
    for item in payload.get("data", {}).get("result", [])[:limit]:
        title = strip_html(item.get("title"))
        arcurl = item.get("arcurl") or item.get("url")
        if not title or not arcurl:
            continue
        candidates.append(
            Candidate(
                title=title,
                url=arcurl,
                source_id=source_id,
                keyword_id=keyword_id,
                author=item.get("author"),
                published_at=datetime.fromtimestamp(item["pubdate"], tz=timezone.utc) if item.get("pubdate") else None,
                snippet=strip_html(item.get("description")),
                raw_payload={"source_type": "bilibili", "query": query, "item": item},
            )
        )
    return candidates
