"""One bounded Bluesky request per operation; no retries or persistent job state."""

import hashlib
import json
import time
from typing import Any, Literal

import httpx
from pydantic import AwareDatetime, BaseModel, Field, ValidationError

from core.clock import utcnow
from sources.schemas import (
    POST_URI,
    SearchInput,
    SocialObject,
    SourceResult,
    ThreadInput,
    UnavailableObject,
)

BASE_URL = "https://public.api.bsky.app/xrpc/"
MAX_BYTES = 2 * 1024 * 1024


class Reference(BaseModel):
    uri: str = Field(pattern=POST_URI, max_length=1024)


class Reply(BaseModel):
    root: Reference
    parent: Reference


class Record(BaseModel):
    text: str = Field(max_length=10000)
    created_at: AwareDatetime = Field(alias="createdAt")
    reply: Reply | None = None


class Author(BaseModel):
    did: str = Field(min_length=1, max_length=1024)


class Post(Reference):
    record: Record
    author: Author
    reply_count: int | None = Field(default=None, ge=0, strict=True, alias="replyCount")


class SearchPage(BaseModel):
    posts: list[Post] = Field(max_length=100)
    cursor: str | None = Field(default=None, min_length=1, max_length=2048)
    hits_total: int | None = Field(default=None, ge=0, strict=True, alias="hitsTotal")


def normalize(post: Post) -> SocialObject:
    reply = post.record.reply
    return SocialObject(
        external_id=post.uri,
        kind="post"
        if reply is None
        else ("comment" if reply.parent.uri == reply.root.uri else "reply"),
        text=post.record.text,
        author_id=post.author.did,
        created_at=post.record.created_at,
        root_id=post.uri if reply is None else reply.root.uri,
        parent_id=None if reply is None else reply.parent.uri,
        reply_count=post.reply_count,
    )


class Bluesky:
    def __init__(self, client: httpx.Client):
        self.client = client

    def _fetch(
        self,
        operation: Literal["search_posts", "fetch_thread"],
        endpoint: str,
        params: dict[str, str | int],
    ) -> tuple[SourceResult, dict[str, Any] | None]:
        result = SourceResult(operation=operation, status="failed", observed_at=utcnow())
        deadline = time.monotonic() + 20
        try:
            with self.client.stream(
                "GET",
                BASE_URL + endpoint,
                params=params,
                follow_redirects=False,
                timeout=httpx.Timeout(10),
                headers={"Accept": "application/json", "User-Agent": "HotKey-Learning/0.2"},
            ) as response:
                result.http_status = response.status_code
                if response.status_code != 200:
                    result.code = {
                        400: "invalid_query",
                        401: "credential_required",
                        403: "access_denied",
                        404: "not_found",
                        429: "rate_limited",
                    }.get(
                        response.status_code,
                        "redirect_blocked" if response.is_redirect else "upstream_unavailable",
                    )
                    retry = response.headers.get("Retry-After", "")
                    if response.status_code == 429 and retry.isascii() and retry.isdigit():
                        result.retry_after_seconds = min(int(retry[:10]), 86400)
                    return result, None
                body = bytearray()
                for chunk in response.iter_bytes(chunk_size=16384):
                    if time.monotonic() >= deadline:
                        result.code = "timeout"
                        return result, None
                    result.response_bytes += len(chunk)
                    if result.response_bytes > MAX_BYTES:
                        result.code = "response_too_large"
                        return result, None
                    body.extend(chunk)
                result.response_sha256 = hashlib.sha256(body).hexdigest()
                data = json.loads(body)
                if not isinstance(data, dict):
                    raise ValueError("expected object")
                return result, data
        except httpx.TimeoutException:
            result.code = "timeout"
        except httpx.HTTPError:
            result.code = "network_error"
        except (ValueError, RecursionError):
            result.code = "schema_changed"
        return result, None

    def search(self, request: SearchInput) -> SourceResult:
        params: dict[str, str | int] = {
            "q": request.keyword,
            "since": request.since.isoformat(),
            "until": request.until.isoformat(),
            "limit": request.limit,
            "sort": "latest",
        }
        if request.cursor is not None:
            params["cursor"] = request.cursor
        result, data = self._fetch("search_posts", "app.bsky.feed.searchPosts", params)
        if data is None:
            return result
        try:
            page = SearchPage.model_validate(data)
            if len(page.posts) > request.limit:
                raise ValueError("page exceeded requested limit")
            result.items = [normalize(post) for post in page.posts]
            result.cursor = page.cursor
            result.total = page.hits_total
            result.status = "ok" if result.items else "empty"
            if page.cursor is not None and page.cursor == request.cursor:
                result.status, result.code, result.cursor = "partial", "cursor_stalled", None
        except (ValidationError, ValueError):
            result.code = "schema_changed"
        return result

    def thread(self, request: ThreadInput) -> SourceResult:
        result, data = self._fetch(
            "fetch_thread",
            "app.bsky.feed.getPostThread",
            {
                "uri": request.uri,
                "depth": request.depth,
                "parentHeight": 0,
            },
        )
        if data is None:
            return result
        try:
            stack: list[tuple[Any, int, str | None]] = [(data["thread"], 0, None)]
            visited: set[str] = set()
            processed = 0
            root_id: str | None = None
            while stack:
                if processed >= request.max_nodes:
                    result.code = "node_limit"
                    break
                node, depth, parent = stack.pop()
                processed += 1
                if not isinstance(node, dict):
                    raise ValueError("invalid thread")
                if node.get("notFound") is True or node.get("blocked") is True:
                    uri = Reference.model_validate(node).uri
                    if depth == 0 and uri != request.uri:
                        raise ValueError("wrong thread")
                    if uri in visited:
                        result.code = "duplicate_node"
                        continue
                    result.unavailable.append(
                        UnavailableObject(
                            external_id=uri,
                            reason="not_found" if node.get("notFound") is True else "blocked",
                        )
                    )
                    visited.add(uri)
                    result.code = "unavailable_nodes"
                    if depth == 0:
                        result.code = "not_found" if node.get("notFound") is True else "blocked"
                        return result
                    continue
                post = Post.model_validate(node["post"])
                if post.uri in visited:
                    result.code = "duplicate_node"
                    continue
                item = normalize(post)
                if (depth == 0 and post.uri != request.uri) or (
                    parent is not None and item.parent_id != parent
                ):
                    raise ValueError("wrong thread relationship")
                if root_id is None:
                    root_id = item.root_id
                elif item.root_id != root_id:
                    raise ValueError("wrong thread root")
                visited.add(post.uri)
                result.items.append(item)
                replies = node.get("replies", [])
                if not isinstance(replies, list):
                    raise ValueError("invalid replies")
                if depth >= request.depth:
                    if replies or item.reply_count is None or item.reply_count > 0:
                        result.code = "depth_limit"
                else:
                    if item.reply_count is not None and item.reply_count > len(replies):
                        result.code = result.code or "incomplete_replies"
                    stack.extend((reply, depth + 1, post.uri) for reply in reversed(replies))
            result.status = "partial" if result.code else ("ok" if result.items else "empty")
        except (KeyError, TypeError, ValueError):
            result.status, result.code = "failed", "schema_changed"
            result.items.clear()
            result.unavailable.clear()
        return result
