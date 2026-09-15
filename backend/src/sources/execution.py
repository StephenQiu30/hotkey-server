import httpx

from sources.adapters.bilibili import Bilibili
from sources.adapters.bluesky import Bluesky
from sources.contracts import FetchedPage
from sources.schemas import BilibiliSearchInput, SearchInput, SearchPageInput


class PublicSearchFetcher:
    def fetch(self, data: SearchPageInput) -> FetchedPage:
        with httpx.Client(trust_env=False) as client:
            if data.source == "bluesky":
                return Bluesky(client).search_page(
                    SearchInput(
                        keyword=data.keyword,
                        since=data.since,
                        until=data.until,
                        limit=data.limit,
                        cursor=data.cursor,
                    )
                )
            if data.source == "bilibili":
                if data.cursor is not None and not data.cursor.isdigit():
                    raise ValueError("invalid bilibili page cursor")
                page = int(data.cursor) if data.cursor is not None else 1
                return Bilibili(client).search_page(
                    BilibiliSearchInput(keyword=data.keyword, page=page, limit=data.limit)
                )
        raise ValueError("source search adapter is unavailable")
