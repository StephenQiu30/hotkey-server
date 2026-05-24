from __future__ import annotations

import asyncio
from datetime import datetime, timezone

import httpx

from server.app.core.settings import settings

from .base import BaseProvider, Candidate, SourceIngestionError
from .registry import register_provider
from .utils import parse_datetime, strip_html

HN_BASE_URL = "https://hacker-news.firebaseio.com/v0"
HN_SEARCH_URL = "https://hn.algolia.com/api/v1/search"


@register_provider("hacker_news", "hn", "hacker-news")
class HackerNewsProvider(BaseProvider):
    source_type = "hacker_news"

    async def fetch_hot_topics(self, query: str | None = None) -> list[Candidate]:
        return await asyncio.to_thread(_fetch_hacker_news, self.source.config, self.source.id, self.keyword.id, query or "")


def _fetch_hacker_news(source_config: dict[str, object], source_id: int, keyword_id: int | None, query: str) -> list[Candidate]:
    limit = int(source_config.get("limit") or settings.source_fetch_limit)
    if source_config.get("frontpage") is True:
        return _fetch_hacker_news_frontpage(source_config, source_id, keyword_id, query, limit)
    params = {"query": query, "tags": "story", "hitsPerPage": limit}
    try:
        response = httpx.get(HN_SEARCH_URL, params=params, timeout=15)
        response.raise_for_status()
        payload = response.json()
    except Exception as exc:  # noqa: BLE001
        raise SourceIngestionError(f"Hacker News fetch failed: {exc}") from exc
    candidates: list[Candidate] = []
    for item in payload.get("hits", [])[:limit]:
        story_id = item.get("objectID")
        title = item.get("title") or item.get("story_title")
        url = item.get("url") or item.get("story_url") or f"https://news.ycombinator.com/item?id={story_id}"
        if not title or not url:
            continue
        candidates.append(
            Candidate(
                title=title,
                url=url,
                source_id=source_id,
                keyword_id=keyword_id,
                author=item.get("author"),
                published_at=parse_datetime(item.get("created_at")),
                snippet=strip_html(item.get("story_text") or item.get("comment_text")),
                raw_payload={"source_type": "hacker_news", "id": story_id, "points": item.get("points"), "query": query, "item": item},
            )
        )
    return candidates


def _fetch_hacker_news_frontpage(source_config: dict[str, object], source_id: int, keyword_id: int | None, query: str, limit: int) -> list[Candidate]:
    endpoint = str(source_config.get("endpoint") or "topstories")
    try:
        with httpx.Client(timeout=15) as client:
            story_ids = client.get(f"{HN_BASE_URL}/{endpoint}.json").raise_for_status().json()
            candidates: list[Candidate] = []
            for story_id in story_ids[:limit]:
                item = client.get(f"{HN_BASE_URL}/item/{story_id}.json").raise_for_status().json()
                title = item.get("title")
                url = item.get("url") or f"https://news.ycombinator.com/item?id={story_id}"
                snippet = item.get("text")
                if not title or not url:
                    continue
                candidates.append(
                    Candidate(
                        title=title,
                        url=url,
                        source_id=source_id,
                        keyword_id=keyword_id,
                        author=item.get("by"),
                        published_at=datetime.fromtimestamp(item["time"], tz=timezone.utc) if item.get("time") else None,
                        snippet=strip_html(snippet),
                        raw_payload={"source_type": "hacker_news", "id": story_id, "score": item.get("score"), "query": query},
                    )
                )
            return candidates
    except Exception as exc:  # noqa: BLE001
        raise SourceIngestionError(f"Hacker News fetch failed: {exc}") from exc
