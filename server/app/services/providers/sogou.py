from __future__ import annotations

import asyncio

import httpx

from .base import BaseProvider, Candidate, SourceIngestionError
from .registry import register_provider


SOGOU_SEARCH_URL = "https://www.sogou.com/web"


@register_provider("sogou", "weibo_sogou", "weibo-sogou")
class SogouProvider(BaseProvider):
    source_type = "sogou"

    async def fetch_hot_topics(self, query: str | None = None) -> list[Candidate]:
        return await asyncio.to_thread(_fetch_sogou, self.source.config, self.source.id, self.keyword.id, query or self.keyword.keyword)


def _fetch_sogou(source_config: dict[str, object], source_id: int, keyword_id: int | None, query: str) -> list[Candidate]:
    limit = int(source_config.get("limit") or 20)
    params = {"query": query}
    headers = {"User-Agent": "Mozilla/5.0"}
    try:
        response = httpx.get(SOGOU_SEARCH_URL, headers=headers, params=params, timeout=20)
        response.raise_for_status()
    except Exception as exc:  # noqa: BLE001
        raise SourceIngestionError(f"Sogou fetch failed: {exc}") from exc

    candidates: list[Candidate] = []
    for index, match in enumerate(response.text.split('href="')[1 : limit + 1], start=1):
        url = match.split('"', 1)[0]
        if not url.startswith("http"):
            continue
        title = f"{query} - Sogou result {index}"
        candidates.append(
            Candidate(
                title=title,
                url=url,
                source_id=source_id,
                keyword_id=keyword_id,
                author="Sogou",
                published_at=None,
                snippet=f"Sogou public search result for {query}.",
                raw_payload={"source_type": "sogou", "query": query, "rank": index},
            )
        )
    return candidates
