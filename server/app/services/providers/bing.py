from __future__ import annotations

import asyncio

import httpx

from server.app.core.settings import settings

from .base import BaseProvider, Candidate, SourceIngestionError
from .registry import register_provider
from .utils import parse_datetime, strip_html


BING_SEARCH_URL = "https://api.bing.microsoft.com/v7.0/search"


@register_provider("bing")
class BingProvider(BaseProvider):
    source_type = "bing"

    async def fetch_hot_topics(self, query: str | None = None) -> list[Candidate]:
        return await asyncio.to_thread(_fetch_bing, self.source.config, self.source.id, self.keyword.id, query or self.keyword.keyword)


def _fetch_bing(source_config: dict[str, object], source_id: int, keyword_id: int | None, query: str) -> list[Candidate]:
    if not settings.bing_search_api_key:
        raise SourceIngestionError(f"Bing fetch skipped for source: BING_SEARCH_API_KEY is not configured.")

    limit = min(int(source_config.get("limit") or settings.source_fetch_limit), 50)
    endpoint = str(source_config.get("endpoint") or BING_SEARCH_URL)
    headers = {"Ocp-Apim-Subscription-Key": settings.bing_search_api_key}
    params = {"q": query, "count": limit, "mkt": source_config.get("mkt") or "zh-CN"}
    try:
        response = httpx.get(endpoint, headers=headers, params=params, timeout=20)
        response.raise_for_status()
        payload = response.json()
    except Exception as exc:  # noqa: BLE001
        raise SourceIngestionError(f"Bing fetch failed for source: {exc}") from exc

    candidates: list[Candidate] = []
    for item in payload.get("webPages", {}).get("value", [])[:limit]:
        title = item.get("name")
        url = item.get("url")
        if not title or not url:
            continue
        candidates.append(
            Candidate(
                title=title,
                url=url,
                source_id=source_id,
                keyword_id=keyword_id,
                author=item.get("siteName"),
                published_at=parse_datetime(item.get("dateLastCrawled")),
                snippet=strip_html(item.get("snippet")),
                raw_payload={"source_type": "bing", "query": query, "item": item},
            )
        )
    return candidates

