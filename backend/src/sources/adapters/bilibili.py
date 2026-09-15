"""Bounded, unauthenticated Bilibili feasibility adapter."""

import hashlib
import json
import re
import time
from datetime import UTC, datetime
from typing import Any

import httpx
from pydantic import BaseModel, Field, ValidationError

from core.clock import utcnow
from sources.schemas import (
    BILIBILI_BVID,
    BilibiliCommentsInput,
    BilibiliPostInput,
    BilibiliRepliesInput,
    BilibiliSearchInput,
    SocialObject,
    SourceReference,
    SourceResult,
)

SEARCH_URL = "https://search.bilibili.com/all"
API_URL = "https://api.bilibili.com"
MAX_BYTES = 2 * 1024 * 1024
BVID_PATTERN = re.compile(rb'bvid:\\?"(BV[1-9A-HJ-NP-Za-km-z]{10})')


class Owner(BaseModel):
    mid: int | str


class Stat(BaseModel):
    reply: int | None = Field(default=None, ge=0)


class Video(BaseModel):
    aid: int = Field(gt=0)
    bvid: str = Field(pattern=BILIBILI_BVID)
    title: str = Field(max_length=1000)
    desc: str = Field(max_length=10000)
    pubdate: int = Field(ge=0)
    owner: Owner
    stat: Stat


class Member(BaseModel):
    mid: int | str


class Content(BaseModel):
    message: str = Field(max_length=10000)


class Reply(BaseModel):
    rpid: int = Field(gt=0)
    root: int = Field(ge=0)
    parent: int = Field(ge=0)
    ctime: int = Field(ge=0)
    rcount: int | None = Field(default=None, ge=0)
    member: Member
    content: Content


def provider_code(value: Any) -> str:
    return {
        -101: "credential_required",
        -352: "access_denied",
        -404: "not_found",
        -412: "access_denied",
    }.get(value, "upstream_rejected")


def object_from_reply(reply: Reply, aid: int, kind: str) -> SocialObject:
    parent = reply.parent or reply.root
    return SocialObject.model_validate(
        {
            "provider_namespace": "comment",
            "external_id": f"comment:{reply.rpid}",
            "kind": kind,
            "text": reply.content.message,
            "author_id": str(reply.member.mid),
            "created_at": datetime.fromtimestamp(reply.ctime, tz=UTC),
            "root_id": f"video:{aid}",
            "parent_id": f"comment:{parent}" if kind == "reply" else None,
            "reply_count": reply.rcount,
        }
    )


class Bilibili:
    def __init__(self, client: httpx.Client):
        self.client = client

    def _fetch(
        self, operation: str, url: str, params: dict[str, str | int]
    ) -> tuple[SourceResult, bytes | None]:
        result = SourceResult.model_validate(
            {
                "source": "bilibili",
                "adapter_version": "bilibili-public-poc-v1",
                "operation": operation,
                "status": "failed",
                "observed_at": utcnow(),
            }
        )
        deadline = time.monotonic() + 20
        try:
            with self.client.stream(
                "GET",
                url,
                params=params,
                follow_redirects=False,
                timeout=httpx.Timeout(10),
                headers={
                    "Accept": "text/html,application/json",
                    "Referer": "https://www.bilibili.com/",
                    "User-Agent": "Mozilla/5.0 HotKey-Source-POC/0.2",
                },
            ) as response:
                result.http_status = response.status_code
                if response.status_code != 200:
                    result.code = {
                        401: "credential_required",
                        403: "access_denied",
                        404: "not_found",
                        412: "access_denied",
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
                payload = bytes(body)
                result.response_sha256 = hashlib.sha256(payload).hexdigest()
                return result, payload
        except httpx.TimeoutException:
            result.code = "timeout"
        except httpx.HTTPError:
            result.code = "network_error"
        return result, None

    @staticmethod
    def _data(result: SourceResult, body: bytes) -> Any | None:
        try:
            envelope = json.loads(body)
            if not isinstance(envelope, dict) or "code" not in envelope:
                raise ValueError("expected provider envelope")
            if envelope["code"] != 0:
                result.code = provider_code(envelope["code"])
                return None
            if "data" not in envelope:
                raise ValueError("missing provider data")
            return envelope["data"]
        except (UnicodeDecodeError, json.JSONDecodeError, ValueError):
            result.code = "schema_changed"
            return None

    def search(self, request: BilibiliSearchInput) -> SourceResult:
        result, body = self._fetch(
            "search_posts",
            SEARCH_URL,
            {"keyword": request.keyword, "page": request.page},
        )
        if body is None:
            return result
        identities = list(dict.fromkeys(match.decode() for match in BVID_PATTERN.findall(body)))
        if not identities:
            result.code = "schema_changed"
            return result
        selected = identities[: request.limit]
        result.references = [
            SourceReference(
                external_id=f"bvid:{identity}",
                canonical_url=f"https://www.bilibili.com/video/{identity}",
            )
            for identity in selected
        ]
        result.cursor = str(request.page + 1) if len(identities) >= request.limit else None
        result.status = "ok"
        return result

    def post(self, request: BilibiliPostInput) -> SourceResult:
        result, body = self._fetch(
            "fetch_post", API_URL + "/x/web-interface/view", {"bvid": request.bvid}
        )
        if body is None:
            return result
        data = self._data(result, body)
        if data is None:
            return result
        try:
            video = Video.model_validate(data)
            result.items = [
                SocialObject(
                    provider_namespace="video",
                    external_id=f"video:{video.aid}",
                    kind="post",
                    text="\n".join(value for value in (video.title, video.desc) if value),
                    author_id=str(video.owner.mid),
                    created_at=datetime.fromtimestamp(video.pubdate, tz=UTC),
                    root_id=f"video:{video.aid}",
                    reply_count=video.stat.reply,
                    canonical_url=f"https://www.bilibili.com/video/{video.bvid}",
                )
            ]
            result.status = "ok"
        except (ValidationError, ValueError, OSError):
            result.code = "schema_changed"
        return result

    def comments(self, request: BilibiliCommentsInput) -> SourceResult:
        result, body = self._fetch(
            "list_comments",
            API_URL + "/x/v2/reply/main",
            {
                "type": 1,
                "oid": request.aid,
                "mode": 3,
                "next": request.cursor,
                "ps": request.limit,
            },
        )
        if body is None:
            return result
        data = self._data(result, body)
        if data is None:
            return result
        try:
            if not isinstance(data, dict):
                raise ValueError("invalid comments data")
            replies = data.get("replies") or []
            if not isinstance(replies, list) or len(replies) > request.limit:
                raise ValueError("invalid comments page")
            parsed = [Reply.model_validate(reply) for reply in replies]
            if any(reply.root or reply.parent for reply in parsed):
                raise ValueError("root page contains nested reply")
            result.items = [object_from_reply(reply, request.aid, "comment") for reply in parsed]
            cursor = data.get("cursor")
            if cursor is not None and not isinstance(cursor, dict):
                raise ValueError("invalid cursor")
            if cursor and cursor.get("is_end") is False:
                next_cursor = cursor.get("next")
                if not isinstance(next_cursor, int) or next_cursor == request.cursor:
                    result.status, result.code = "partial", "cursor_stalled"
                    return result
                result.cursor = str(next_cursor)
            result.status = "ok" if result.items else "empty"
        except (ValidationError, ValueError, OSError):
            result.items = []
            result.code = "schema_changed"
        return result

    def replies(self, request: BilibiliRepliesInput) -> SourceResult:
        result, body = self._fetch(
            "list_replies",
            API_URL + "/x/v2/reply/reply",
            {
                "type": 1,
                "oid": request.aid,
                "root": request.root_id,
                "pn": request.page,
                "ps": request.limit,
            },
        )
        if body is None:
            return result
        data = self._data(result, body)
        if data is None:
            return result
        try:
            if not isinstance(data, dict):
                raise ValueError("invalid replies data")
            replies = data.get("replies") or []
            if not isinstance(replies, list) or len(replies) > request.limit:
                raise ValueError("invalid replies page")
            parsed = [Reply.model_validate(reply) for reply in replies]
            if any(reply.root != request.root_id for reply in parsed):
                raise ValueError("wrong reply root")
            result.items = [object_from_reply(reply, request.aid, "reply") for reply in parsed]
            page = data.get("page")
            if page is not None and not isinstance(page, dict):
                raise ValueError("invalid page")
            if page:
                number, size, count = page.get("num"), page.get("size"), page.get("count")
                if (
                    not isinstance(number, int)
                    or not isinstance(size, int)
                    or not isinstance(count, int)
                ):
                    raise ValueError("invalid page counts")
                if number * size < count:
                    result.cursor = str(number + 1)
            result.status = "ok" if result.items else "empty"
        except (ValidationError, ValueError, OSError):
            result.items = []
            result.code = "schema_changed"
        return result
