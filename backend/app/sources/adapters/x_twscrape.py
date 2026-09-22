from __future__ import annotations

import asyncio
import json
import os
import threading
import time
from collections.abc import Callable
from contextlib import suppress
from datetime import UTC, datetime
from typing import Any

import httpx
from bs4 import BeautifulSoup
from pydantic import BaseModel, ConfigDict, SecretStr, field_validator
from twscrape.account import TOKEN
from twscrape.api import GQL_FEATURES, GQL_URL, OP_SearchTimeline, OP_TweetDetail, OP_UserTweets
from twscrape.logger import logger as sdk_logger
from twscrape.models import Tweet
from twscrape.utils import get_by_path, to_old_rep
from twscrape.xclid import XClIdAccountError, XClIdGen, XClIdParseError, load_keys

from sources.contracts import (
    AuthorPostsRequest,
    CommentsRequest,
    RepliesRequest,
    SearchRequest,
    SourceCapability,
    SourceComment,
    SourceItem,
    SourcePage,
    SourcePageState,
    SourcePost,
    SourceRequest,
    SourceStopReason,
)

_VERSION = "twscrape/0.20.1"
_MAX_RESPONSE_BYTES = 4 * 1024 * 1024


class XSession(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    auth_token: SecretStr
    csrf_token: SecretStr

    @field_validator("auth_token", "csrf_token")
    @classmethod
    def validate_cookie(cls, value: SecretStr) -> SecretStr:
        raw = value.get_secret_value()
        if (
            not raw
            or len(raw) > 4096
            or any(ord(char) < 33 or ord(char) > 126 or char == ";" for char in raw)
        ):
            raise ValueError("invalid session cookie")
        return value


class _SourceStoppedError(Exception):
    def __init__(self, reason: SourceStopReason, retry_at: datetime | None = None) -> None:
        super().__init__(reason.value)
        self.reason = reason
        self.retry_at = retry_at


def _classify_response(response: httpx.Response) -> None:
    status = response.status_code
    reason = {
        401: SourceStopReason.AUTHENTICATION_REQUIRED,
        403: SourceStopReason.ACCESS_DENIED,
        404: SourceStopReason.NOT_FOUND,
        429: SourceStopReason.RATE_LIMITED,
    }.get(status)
    if reason is not None:
        retry_at = None
        if status == 429:
            with suppress(KeyError, ValueError, OverflowError, OSError):
                retry_at = datetime.fromtimestamp(int(response.headers["x-rate-limit-reset"]), UTC)
        raise _SourceStoppedError(reason, retry_at)
    if status >= 500:
        raise _SourceStoppedError(SourceStopReason.UPSTREAM_ERROR)
    if status != 200:
        raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)


class _MeteredClient:
    """The SDK signing helpers and timeline reads share this one request boundary."""

    def __init__(self, adapter: XTwscrapeAdapter, client: httpx.AsyncClient) -> None:
        self.adapter = adapter
        self.client = client

    async def get(self, url: str, **kwargs: Any) -> httpx.Response:
        try:
            return await self._get(url, **kwargs)
        except _SourceStoppedError as error:
            self.adapter._boundary_stop = error
            raise

    async def _get(self, url: str, **kwargs: Any) -> httpx.Response:
        if self.adapter._boundary_stop is not None:
            raise self.adapter._boundary_stop
        parsed = httpx.URL(url)
        if (
            parsed.scheme != "https"
            or parsed.host not in {"x.com", "abs.twimg.com"}
            or parsed.port not in {None, 443}
            or parsed.userinfo
        ):
            raise _SourceStoppedError(SourceStopReason.ACCESS_DENIED)
        self.adapter._check_budget()
        attempt = self.adapter._request_count + 1
        if not self.adapter._before_request(attempt):
            raise _SourceStoppedError(SourceStopReason.BUDGET_EXHAUSTED)
        self.adapter._request_count = attempt
        headers = dict(kwargs.pop("headers", {}))
        session = self.adapter._session
        if parsed.host == "x.com" and session is not None:
            headers.update(
                {
                    "cookie": (
                        f"auth_token={session.auth_token.get_secret_value()}; "
                        f"ct0={session.csrf_token.get_secret_value()}"
                    ),
                    "x-csrf-token": session.csrf_token.get_secret_value(),
                    "authorization": TOKEN,
                    "x-twitter-active-user": "yes",
                    "x-twitter-client-language": "en",
                }
            )
        async with self.client.stream("GET", url, headers=headers, **kwargs) as response:
            _classify_response(response)
            content = bytearray()
            async for chunk in response.aiter_bytes():
                self.adapter._check_cancelled()
                content.extend(chunk)
                if len(content) > _MAX_RESPONSE_BYTES:
                    raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
            return httpx.Response(
                response.status_code,
                headers=response.headers,
                content=bytes(content),
                request=response.request,
            )


class XTwscrapeAdapter:
    """One bounded collection run. The caller owns authorization, lease and durable budget."""

    source_key = "x"
    capabilities = frozenset(SourceCapability)

    def __init__(
        self,
        *,
        session: XSession | None,
        before_request: Callable[[int], bool],
        max_requests: int = 20,
        max_seconds: float = 90,
        max_empty_pages: int = 3,
        cancelled: Callable[[], bool] = lambda: False,
        transport: httpx.AsyncBaseTransport | None = None,
        transaction_id: Callable[[str, str], str] | None = None,
    ) -> None:
        if max_requests < 1 or not 0 < max_seconds <= 90 or max_empty_pages < 1:
            raise ValueError("invalid source request limits")
        os.environ["TWS_TELEMETRY"] = "0"
        sdk_logger.disable("twscrape")
        self._session = session
        self._before_request = before_request
        self._max_requests = max_requests
        self._max_seconds = max_seconds
        self._max_empty_pages = max_empty_pages
        self._cancelled = cancelled
        self._transport = transport
        self._transaction_id = transaction_id
        self._request_count = 0
        self._deadline: float | None = None
        self._seen_cursors: set[str] = set()
        self._empty_pages = 0
        self._lock = threading.Lock()
        self._request_scope: dict[str, Any] | None = None
        self._terminal: tuple[SourceStopReason, datetime | None] | None = None
        self._boundary_stop: _SourceStoppedError | None = None
        self._completed_page: SourcePage | None = None

    def _check_cancelled(self) -> None:
        if self._cancelled():
            raise _SourceStoppedError(SourceStopReason.CANCELLED)

    def _check_budget(self) -> None:
        self._check_cancelled()
        if self._request_count >= self._max_requests or (
            self._deadline is not None and time.monotonic() >= self._deadline
        ):
            raise _SourceStoppedError(SourceStopReason.BUDGET_EXHAUSTED)

    def fetch_page(self, request: SourceRequest) -> SourcePage:
        if not self._lock.acquire(blocking=False):
            raise ValueError("a source adapter cannot be shared between concurrent runs")
        start_count = self._request_count
        try:
            if self._deadline is None:
                self._deadline = time.monotonic() + self._max_seconds
            scope = request.model_dump(exclude={"page_token", "page_size"})
            if self._request_scope is not None and scope != self._request_scope:
                raise ValueError("a source adapter belongs to one collection scope")
            self._request_scope = scope
            try:
                if self._completed_page is not None:
                    return self._completed_page.model_copy(update={"request_count": 0})
                if self._terminal is not None:
                    raise _SourceStoppedError(*self._terminal)
                self._check_budget()
                if request.source_key != "x" or request.watermark is not None:
                    raise _SourceStoppedError(SourceStopReason.UNSUPPORTED)
                if self._session is None or not (
                    self._session.auth_token.get_secret_value()
                    and self._session.csrf_token.get_secret_value()
                ):
                    raise _SourceStoppedError(SourceStopReason.AUTHENTICATION_REQUIRED)
                page = asyncio.run(self._fetch_with_cancellation(request))
            except _SourceStoppedError as error:
                page = self._page(request, (), None, error.reason, error.retry_at)
            except (httpx.HTTPError, TimeoutError):
                page = self._page(request, (), None, SourceStopReason.UPSTREAM_ERROR)
            except (ValueError, KeyError, TypeError, IndexError, AttributeError, XClIdParseError):
                page = self._page(request, (), None, SourceStopReason.PROTOCOL_ERROR)
            except XClIdAccountError:
                page = self._page(request, (), None, SourceStopReason.AUTHENTICATION_REQUIRED)
            if self._boundary_stop is not None:
                page = self._page(
                    request, (), None, self._boundary_stop.reason, self._boundary_stop.retry_at
                )
            if page.state in {SourcePageState.STOPPED, SourcePageState.PARTIAL}:
                assert page.stop_reason is not None
                self._terminal = (page.stop_reason, page.retry_at)
            elif page.state in {SourcePageState.COMPLETE, SourcePageState.EMPTY}:
                self._completed_page = page
            return page.model_copy(update={"request_count": self._request_count - start_count})
        finally:
            self._lock.release()

    async def _fetch_with_cancellation(self, request: SourceRequest) -> SourcePage:
        task = asyncio.create_task(self._fetch_page(request))
        try:
            while not task.done():
                self._check_cancelled()
                await asyncio.wait({task}, timeout=0.05)
            return await task
        finally:
            if not task.done():
                task.cancel()
            await asyncio.gather(task, return_exceptions=True)

    async def _fetch_page(self, request: SourceRequest) -> SourcePage:
        assert self._deadline is not None
        operation, variables = self._operation(request)
        try:
            async with asyncio.timeout(max(0, self._deadline - time.monotonic())):
                async with httpx.AsyncClient(
                    transport=self._transport or httpx.AsyncHTTPTransport(retries=0),
                    timeout=httpx.Timeout(10, connect=5, pool=5),
                    follow_redirects=False,
                    trust_env=False,
                ) as client:
                    bounded = _MeteredClient(self, client)
                    if self._transaction_id is None:
                        homepage = await bounded.get("https://x.com/")
                        keys, animation = await load_keys(
                            BeautifulSoup(homepage.text, "html.parser"), bounded
                        )
                        self._transaction_id = XClIdGen(keys, animation).calc
                    url = f"{GQL_URL}/{operation}"
                    params = {
                        "variables": json.dumps(variables),
                        "features": json.dumps(GQL_FEATURES),
                    }
                    if isinstance(request, SearchRequest):
                        params["fieldToggles"] = json.dumps({"withArticleRichContentState": False})
                    assert self._transaction_id is not None
                    response = await bounded.get(
                        url,
                        params=params,
                        headers={
                            "x-client-transaction-id": self._transaction_id(
                                "GET", httpx.URL(url).path
                            )
                        },
                    )
                    self._check_cancelled()
                    return self._parse_page(request, response.json())
        except TimeoutError:
            raise _SourceStoppedError(SourceStopReason.BUDGET_EXHAUSTED) from None

    @staticmethod
    def _operation(request: SourceRequest) -> tuple[str, dict[str, Any]]:
        if isinstance(request, SearchRequest):
            operation = OP_SearchTimeline
            variables: dict[str, Any] = {
                "rawQuery": request.query,
                "product": request.sort.value.title(),
                "querySource": "typed_query",
                "count": request.page_size,
            }
        elif isinstance(request, AuthorPostsRequest):
            if not request.author_external_id.isdecimal():
                raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
            operation = OP_UserTweets
            variables = {
                "userId": request.author_external_id,
                "count": request.page_size,
                "includePromotedContent": False,
                "withVoice": True,
                "withV2Timeline": True,
            }
        else:
            target = request.post_external_id
            if target is None or not target.isdecimal():
                raise _SourceStoppedError(SourceStopReason.UNSUPPORTED)
            operation = OP_TweetDetail
            variables = {
                "focalTweetId": target,
                "referrer": "profile",
                "with_rux_injections": False,
                "includePromotedContent": False,
                "withCommunity": True,
                "withVoice": True,
                "withV2Timeline": True,
            }
        if request.page_token is not None:
            variables["cursor"] = request.page_token
        return operation, variables

    def _parse_page(self, request: SourceRequest, data: Any) -> SourcePage:
        if not isinstance(data, dict):
            raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
        errors = data.get("errors", [])
        if errors:
            codes = {error.get("code") for error in errors if isinstance(error, dict)}
            reason = (
                SourceStopReason.AUTHENTICATION_REQUIRED
                if codes & {32, 89, 215}
                else SourceStopReason.ACCESS_DENIED
                if 326 in codes
                else SourceStopReason.RATE_LIMITED
                if 88 in codes
                else SourceStopReason.PROTOCOL_ERROR
            )
            raise _SourceStoppedError(reason)
        entries = get_by_path(data, "entries")
        if not isinstance(entries, list):
            raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
        items: list[SourceItem] = []
        seen: set[str] = set()
        cursor: str | None = None
        for entry in entries:
            if not isinstance(entry, dict) or not isinstance(entry.get("content"), dict):
                raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
            content = entry["content"]
            if content.get("cursorType") in {"Bottom", "ShowMoreThreads"}:
                cursor = content.get("value")
                if not isinstance(cursor, str) or not cursor:
                    raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
                continue
            module = content.get("items")
            candidates = (
                [part.get("item", {}) for part in module] if isinstance(module, list) else [content]
            )
            for candidate in candidates:
                item_content = candidate.get("itemContent", {})
                if item_content.get("promotedMetadata") is not None:
                    continue
                result = item_content.get("tweet_results", {}).get("result")
                if not isinstance(result, dict):
                    if str(entry.get("entryId", "")).startswith(("tweet-", "conversationthread-")):
                        raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
                    continue
                if result.get("__typename") == "TweetWithVisibilityResults":
                    result = result["tweet"]
                item = self._map_tweet(request, result)
                if item is not None and item.external_id not in seen:
                    items.append(item)
                    seen.add(item.external_id)
        if len(items) > request.page_size:
            return self._page(
                request, tuple(items[: request.page_size]), None, SourceStopReason.BUDGET_EXHAUSTED
            )
        if request.page_token is not None:
            self._seen_cursors.add(request.page_token)
        if cursor is not None and cursor in self._seen_cursors:
            return self._page(request, tuple(items), None, SourceStopReason.PROTOCOL_ERROR)
        self._empty_pages = self._empty_pages + 1 if not items else 0
        if cursor is not None and self._empty_pages >= self._max_empty_pages:
            return self._page(request, (), None, SourceStopReason.PROTOCOL_ERROR)
        return self._page(request, tuple(items), cursor, None)

    @staticmethod
    def _map_tweet(request: SourceRequest, raw: dict[str, Any]) -> SourceItem | None:
        if raw.get("__typename") != "Tweet":
            raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
        fields = {**(raw.get("legacy") or {}), **raw}
        if "full_text" not in fields or "conversation_id_str" not in fields:
            raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
        normalized = to_old_rep(raw)
        identifier = raw["rest_id"]
        # Direct parsing raises; the SDK convenience generator would dump and skip failures.
        tweet = Tweet.parse(normalized["tweets"][identifier], normalized)
        body = get_by_path(raw.get("note_tweet", {}), "text") or fields["full_text"]
        values = {
            "source_key": "x",
            "external_id": identifier,
            "author_external_id": str(tweet.user.id),
            "published_at": tweet.date,
            "text": body,
            "like_count": fields.get("favorite_count"),
        }
        if isinstance(request, (CommentsRequest, RepliesRequest)):
            root = request.post_external_id
            parent = fields.get("in_reply_to_status_id_str")
            if identifier == root or fields["conversation_id_str"] != root or parent is None:
                return None
            if isinstance(request, RepliesRequest) and parent != request.comment_external_id:
                return None
            return SourceComment.model_validate(
                {
                    **values,
                    "post_external_id": root,
                    "parent_comment_external_id": None if parent == root else parent,
                }
            )
        if (
            isinstance(request, AuthorPostsRequest)
            and str(tweet.user.id) != request.author_external_id
        ):
            raise _SourceStoppedError(SourceStopReason.PROTOCOL_ERROR)
        return SourcePost.model_validate(
            {
                **values,
                "comment_count": fields.get("reply_count"),
                "repost_count": fields.get("retweet_count"),
                "canonical_url": f"https://x.com/i/status/{identifier}",
                "conversation_external_id": fields["conversation_id_str"],
                "parent_external_id": fields.get("in_reply_to_status_id_str"),
                "quote_external_id": fields.get("quoted_status_id_str")
                or get_by_path(fields.get("quoted_status_result", {}), "rest_id"),
                "repost_external_id": fields.get("retweeted_status_id_str")
                or get_by_path(fields.get("retweeted_status_result", {}), "rest_id"),
                "text_scope": "truncated" if fields.get("truncated") else "full",
            }
        )

    @staticmethod
    def _page(
        request: SourceRequest,
        items: tuple[SourceItem, ...],
        cursor: str | None,
        reason: SourceStopReason | None,
        retry_at: datetime | None = None,
    ) -> SourcePage:
        if reason is not None:
            state = SourcePageState.PARTIAL if items else SourcePageState.STOPPED
        elif cursor is not None:
            state = SourcePageState.MORE
        elif items:
            state, reason = SourcePageState.COMPLETE, SourceStopReason.END_OF_RESULTS
        else:
            state, reason = SourcePageState.EMPTY, SourceStopReason.SOURCE_EMPTY
        return SourcePage(
            source_key="x",
            capability=request.capability,
            state=state,
            items=items,
            next_page_token=cursor,
            watermark=None,
            stop_reason=reason,
            observed_at=datetime.now(UTC),
            adapter_version=_VERSION,
            retry_at=retry_at,
        )
