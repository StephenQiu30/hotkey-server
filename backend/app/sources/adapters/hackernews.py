from __future__ import annotations

import json
from collections.abc import Callable
from typing import Any, ClassVar
from urllib.parse import urlsplit

import httpx

from sources.adapters.http_source import (
    HttpSourceAdapter,
    SourceFailureError,
    html_to_text,
    parse_timestamp,
)
from sources.contracts import (
    CommentsRequest,
    SearchRequest,
    SocialSourceCapability,
    SourceCapability,
    SourceComment,
    SourcePage,
    SourcePost,
    SourceRequest,
    SourceSort,
    SourceStopReason,
)

_API = "https://hn.algolia.com/api/v1"
_ITEM_URL = "https://news.ycombinator.com/item?id={id}"
_MAX_PAGES = 50


def _hn_id(value: object) -> str:
    text = str(value) if isinstance(value, int) and not isinstance(value, bool) else value
    if not isinstance(text, str) or not text.isascii() or not text.isdigit() or len(text) > 12:
        raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
    return text


def _count(value: object) -> int | None:
    return value if isinstance(value, int) and not isinstance(value, bool) and value >= 0 else None


class HackerNewsAdapter(HttpSourceAdapter):
    """Hacker News through the public Algolia API: story search and full comment trees.

    Comments of one story are fetched once as a tree, flattened in reply order and served
    in pages whose token is the next offset; each comment keeps its parent comment ID.
    """

    capabilities: ClassVar[frozenset[SocialSourceCapability]] = frozenset(
        {SourceCapability.SEARCH, SourceCapability.COMMENTS}
    )
    adapter_version: ClassVar[str] = "hn-algolia-v1"

    def __init__(
        self,
        *,
        before_request: Callable[[int], bool],
        base_url: str = _API,
        allowed_hosts: frozenset[str] = frozenset({"hn.algolia.com"}),
        cancelled: Callable[[], bool] = lambda: False,
        max_requests: int = 10,
        max_seconds: float = 60,
        transport: httpx.BaseTransport | None = None,
    ) -> None:
        parsed = urlsplit(base_url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError("base_url must be an http(s) URL")
        super().__init__(
            source_key="hackernews",
            allowed_hosts=allowed_hosts,
            before_request=before_request,
            cancelled=cancelled,
            max_requests=max_requests,
            max_seconds=max_seconds,
            transport=transport,
        )
        if parsed.hostname not in self._allowed_hosts:
            raise ValueError("base_url host must be allowlisted")
        self._api = base_url.rstrip("/")
        self._comment_cache: tuple[SourceComment, ...] | None = None

    def _fetch(self, request: SourceRequest) -> SourcePage:
        if isinstance(request, SearchRequest):
            return self._search(request)
        if isinstance(request, CommentsRequest):
            return self._comments(request)
        raise SourceFailureError(SourceStopReason.UNSUPPORTED)

    def _json(self, url: str, params: dict[str, str] | None = None) -> Any:
        return json.loads(self._get_bytes(url, params=params))

    def _search(self, request: SearchRequest) -> SourcePage:
        page_index = 0
        if request.page_token is not None:
            if not request.page_token.isdigit():
                raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
            page_index = int(request.page_token)
        params = {
            "query": request.query,
            "tags": "story",
            "hitsPerPage": str(request.page_size),
            "page": str(page_index),
        }
        if request.starts_at is not None and request.ends_at is not None:
            params["numericFilters"] = (
                f"created_at_i>={int(request.starts_at.timestamp())},"
                f"created_at_i<{int(request.ends_at.timestamp())}"
            )
        endpoint = "search_by_date" if request.sort is SourceSort.LATEST else "search"
        payload = self._json(f"{self._api}/{endpoint}", params)
        if not isinstance(payload, dict) or not isinstance(payload.get("hits"), list):
            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        items = tuple(self._post(hit) for hit in payload["hits"][: request.page_size])
        pages = payload.get("nbPages")
        has_more = (
            isinstance(pages, int)
            and page_index + 1 < min(pages, _MAX_PAGES)
            and len(payload["hits"]) >= request.page_size
        )
        return self._page(request, items, str(page_index + 1) if has_more else None)

    def _post(self, hit: object) -> SourcePost:
        if not isinstance(hit, dict):
            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        identifier = _hn_id(hit.get("objectID"))
        author = hit.get("author") if isinstance(hit.get("author"), str) else None
        title = hit.get("title") if isinstance(hit.get("title"), str) else None
        story_url = hit.get("url") if isinstance(hit.get("url"), str) else None
        text = html_to_text(hit.get("story_text")) or story_url
        return SourcePost(
            source_key=self.source_key,
            external_id=identifier,
            author_external_id=author or None,
            author_name=author or None,
            published_at=parse_timestamp(hit.get("created_at_i") or hit.get("created_at")),
            title=title or None,
            text=text,
            like_count=_count(hit.get("points")),
            comment_count=_count(hit.get("num_comments")),
            repost_count=None,
            canonical_url=_ITEM_URL.format(id=identifier),
        )

    def _comments(self, request: CommentsRequest) -> SourcePage:
        story_id = _hn_id(request.post_external_id)
        offset = 0
        if request.page_token is not None:
            if not request.page_token.isdigit():
                raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
            offset = int(request.page_token)
        if self._comment_cache is None:
            tree = self._json(f"{self._api}/items/{story_id}")
            if not isinstance(tree, dict) or _hn_id(tree.get("id")) != story_id:
                raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
            flattened: list[SourceComment] = []
            self._flatten(tree.get("children"), story_id, None, flattened)
            self._comment_cache = tuple(flattened)
        page = self._comment_cache[offset : offset + request.page_size]
        next_offset = offset + len(page)
        return self._page(
            request,
            page,
            str(next_offset) if next_offset < len(self._comment_cache) else None,
        )

    def _flatten(
        self,
        children: object,
        story_id: str,
        parent_id: str | None,
        out: list[SourceComment],
    ) -> None:
        if children is None:
            return
        if not isinstance(children, list):
            raise SourceFailureError(SourceStopReason.PROTOCOL_ERROR)
        for child in children:
            if not isinstance(child, dict) or child.get("type") != "comment":
                continue
            identifier = _hn_id(child.get("id"))
            text = html_to_text(child.get("text"))
            author = child.get("author") if isinstance(child.get("author"), str) else None
            if text is not None:
                out.append(
                    SourceComment(
                        source_key=self.source_key,
                        external_id=identifier,
                        post_external_id=story_id,
                        parent_comment_external_id=parent_id,
                        author_external_id=author or None,
                        author_name=author or None,
                        published_at=parse_timestamp(
                            child.get("created_at_i") or child.get("created_at")
                        ),
                        text=text,
                        like_count=_count(child.get("points")),
                        canonical_url=_ITEM_URL.format(id=identifier),
                    )
                )
            self._flatten(child.get("children"), story_id, identifier, out)
