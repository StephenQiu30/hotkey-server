from __future__ import annotations

import asyncio
from xml.etree import ElementTree

import httpx

from server.app.core.settings import settings

from .base import BaseProvider, Candidate, SourceIngestionError
from .registry import register_provider
from .utils import normalize_source_url, parse_datetime, rss_link, strip_html, xml_text


DEFAULT_RSS_URL = "https://hnrss.org/frontpage"


@register_provider("rss")
class RssProvider(BaseProvider):
    source_type = "rss"

    async def fetch_hot_topics(self, query: str | None = None) -> list[Candidate]:
        return await asyncio.to_thread(_fetch_rss, self.source.config, self.source.id, self.keyword.id, query)


def _fetch_rss(source_config: dict[str, object], source_id: int, keyword_id: int | None, query: str | None) -> list[Candidate]:
    url = normalize_source_url(str(source_config.get("url") or DEFAULT_RSS_URL))
    if not url:
        raise SourceIngestionError("RSS source missing feed url")
    limit = int(source_config.get("limit") or settings.source_fetch_limit)
    try:
        response = httpx.get(url, timeout=15)
        response.raise_for_status()
        root = ElementTree.fromstring(response.text)
    except Exception as exc:  # noqa: BLE001
        raise SourceIngestionError(f"RSS fetch failed: {exc}") from exc

    items = root.findall(".//item") or root.findall(".//{http://www.w3.org/2005/Atom}entry")
    candidates: list[Candidate] = []
    for item in items[:limit]:
        title = xml_text(item, "title")
        link = rss_link(item)
        snippet = xml_text(item, "description") or xml_text(item, "summary")
        author = xml_text(item, "author") or xml_text(item, "{http://purl.org/dc/elements/1.1/}creator")
        published = parse_datetime(xml_text(item, "pubDate") or xml_text(item, "published") or xml_text(item, "updated"))
        if not title or not link:
            continue
        candidates.append(
            Candidate(
                title=title,
                url=link,
                source_id=source_id,
                keyword_id=keyword_id,
                author=author,
                published_at=published,
                snippet=strip_html(snippet),
                raw_payload={"source_type": "rss", "feed_url": url, "query": query},
            )
        )
    return candidates

