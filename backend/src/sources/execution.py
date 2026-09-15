import httpx

from sources.adapters.bilibili import Bilibili
from sources.adapters.bluesky import Bluesky
from sources.contracts import FetchedPage
from sources.schemas import (
    BilibiliCommentsInput,
    BilibiliPostInput,
    BilibiliRepliesInput,
    BilibiliSearchInput,
    CollectionPageInput,
    SearchInput,
)


class PublicCollectionFetcher:
    def fetch(self, data: CollectionPageInput) -> FetchedPage:
        with httpx.Client(trust_env=False) as client:
            if data.source == "bluesky" and data.operation == "search_posts":
                return Bluesky(client).search_page(
                    SearchInput(
                        keyword=data.request_value,
                        since=data.since,
                        until=data.until,
                        limit=data.limit,
                    )
                )
            if data.source == "bilibili" and data.operation == "search_posts":
                return Bilibili(client).search_page(
                    BilibiliSearchInput(keyword=data.request_value, page=1, limit=data.limit)
                )
            if data.source == "bilibili" and data.operation == "fetch_post":
                return Bilibili(client).post_page(
                    BilibiliPostInput(bvid=data.request_value.removeprefix("bvid:"))
                )
            if data.source == "bilibili" and data.operation == "list_comments":
                return Bilibili(client).comments_page(
                    BilibiliCommentsInput(
                        aid=int(data.request_value.removeprefix("aid:")),
                        cursor=0,
                        limit=data.limit,
                    )
                )
            if data.source == "bilibili" and data.operation == "list_replies":
                aid, root = data.request_value.split("/")
                return Bilibili(client).replies_page(
                    BilibiliRepliesInput(
                        aid=int(aid.removeprefix("aid:")),
                        root_id=int(root.removeprefix("root:")),
                        page=1,
                        limit=data.limit,
                    )
                )
        raise ValueError("source operation adapter is unavailable")
