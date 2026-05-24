from __future__ import annotations

import asyncio

import httpx

from server.app.core.settings import settings

from .base import BaseProvider, Candidate, SourceIngestionError
from .registry import register_provider
from .utils import parse_datetime


X_SEARCH_URL = "https://api.x.com/2/tweets/search/recent"


@register_provider("x", "twitter", "x_twitter", "x-twitter")
class XTwitterProvider(BaseProvider):
    source_type = "x_twitter"

    async def fetch_hot_topics(self, query: str | None = None) -> list[Candidate]:
        return await asyncio.to_thread(_fetch_x_twitter, self.source.config, self.source.id, self.keyword.id, query or self.keyword.keyword)


def _fetch_x_twitter(source_config: dict[str, object], source_id: int, keyword_id: int | None, query: str) -> list[Candidate]:
    if not settings.x_api_bearer_token:
        raise SourceIngestionError(f"X/Twitter fetch skipped for source: X_API_BEARER_TOKEN is not configured.")

    limit = max(10, min(int(source_config.get("limit") or settings.source_fetch_limit), 100))
    search_query = str(source_config.get("query_template") or f"{query} -is:retweet -is:reply")
    params = {
        "query": search_query,
        "max_results": limit,
        "tweet.fields": "id,text,created_at,author_id,public_metrics,lang,source",
        "expansions": "author_id",
        "user.fields": "id,name,username,verified",
    }
    headers = {"Authorization": f"Bearer {settings.x_api_bearer_token}"}
    try:
        response = httpx.get(X_SEARCH_URL, headers=headers, params=params, timeout=20)
        response.raise_for_status()
        payload = response.json()
    except Exception as exc:  # noqa: BLE001
        raise SourceIngestionError(f"X/Twitter fetch failed for source: {exc}") from exc

    users = {user.get("id"): user for user in payload.get("includes", {}).get("users", [])}
    candidates: list[Candidate] = []
    for item in payload.get("data", []):
        user = users.get(item.get("author_id"), {})
        username = user.get("username") or item.get("author_id") or "unknown"
        tweet_id = item.get("id")
        text = item.get("text")
        if not tweet_id or not text:
            continue
        candidates.append(
            Candidate(
                title=text[:120],
                url=f"https://x.com/{username}/status/{tweet_id}",
                source_id=source_id,
                keyword_id=keyword_id,
                author=username,
                published_at=parse_datetime(item.get("created_at")),
                snippet=text,
                raw_payload={"source_type": "x_twitter", "query": search_query, "tweet": item, "user": user},
            )
        )
    return candidates
