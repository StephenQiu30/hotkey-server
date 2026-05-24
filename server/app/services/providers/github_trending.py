from __future__ import annotations

import asyncio

import httpx

from server.app.core.settings import settings

from .base import BaseProvider, Candidate, SourceIngestionError
from .registry import register_provider
from .utils import parse_datetime

GITHUB_SEARCH_REPOSITORIES_URL = "https://api.github.com/search/repositories"


@register_provider("github_trending", "github-trending", "github", "github_repositories")
class GitHubTrendingProvider(BaseProvider):
    source_type = "github_trending"

    async def fetch_hot_topics(self, query: str | None = None) -> list[Candidate]:
        return await asyncio.to_thread(_fetch_github_trending, self.source.config, self.source.id, self.keyword.id, query or self.keyword.keyword)


def _fetch_github_trending(source_config: dict[str, object], source_id: int, keyword_id: int | None, query: str | None) -> list[Candidate]:
    limit = max(1, min(int(source_config.get("limit") or settings.source_fetch_limit), 100))
    search_query = _build_search_query(source_config, query)
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "hotkey-server"}
    token = str(source_config.get("token") or "").strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"

    params = {
        "q": search_query,
        "sort": str(source_config.get("sort") or "stars"),
        "order": str(source_config.get("order") or "desc"),
        "per_page": limit,
    }
    try:
        response = httpx.get(GITHUB_SEARCH_REPOSITORIES_URL, params=params, headers=headers, timeout=15)
        response.raise_for_status()
        payload = response.json()
    except Exception as exc:  # noqa: BLE001
        raise SourceIngestionError(f"GitHub trending fetch failed: {exc}") from exc

    candidates: list[Candidate] = []
    for item in payload.get("items", [])[:limit]:
        title = item.get("full_name") or item.get("name")
        url = item.get("html_url")
        if not title or not url:
            continue
        owner = item.get("owner") if isinstance(item.get("owner"), dict) else {}
        candidates.append(
            Candidate(
                title=str(title),
                url=str(url),
                source_id=source_id,
                keyword_id=keyword_id,
                author=owner.get("login"),
                published_at=parse_datetime(item.get("pushed_at") or item.get("updated_at") or item.get("created_at")),
                snippet=item.get("description"),
                raw_payload={
                    "source_type": "github_trending",
                    "id": item.get("id"),
                    "stars": item.get("stargazers_count"),
                    "forks": item.get("forks_count"),
                    "language": item.get("language"),
                    "topics": item.get("topics") or [],
                    "query": query,
                    "item": item,
                },
            )
        )
    return candidates


def _build_search_query(source_config: dict[str, object], query: str | None) -> str:
    parts: list[str] = []
    normalized_query = (query or "").strip()
    if normalized_query:
        parts.append(f"{normalized_query} in:name,description,readme")
    else:
        parts.append("stars:>=100")

    language = str(source_config.get("language") or "").strip()
    if language:
        parts.append(f"language:{language}")

    min_stars = source_config.get("min_stars")
    if min_stars is not None:
        parts.append(f"stars:>={int(min_stars)}")

    pushed_since = str(source_config.get("pushed_since") or "").strip()
    if pushed_since:
        parts.append(f"pushed:>={pushed_since}")

    return " ".join(parts)
