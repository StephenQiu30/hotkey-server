from __future__ import annotations

import asyncio
import json
import logging
import os
from collections.abc import Callable

import httpx
import pytest
from pydantic import SecretStr

from core.logging import configure_logging
from sources.adapters.x_twscrape import XSession, XTwscrapeAdapter
from sources.contracts import (
    AuthorPostsRequest,
    CommentsRequest,
    RepliesRequest,
    SearchRequest,
    SourcePageState,
    SourceSort,
    SourceStopReason,
)


def _tweet(identifier: str = "9007199254740993", *, author: str = "42", **fields: object) -> dict:
    return {
        "__typename": "Tweet",
        "rest_id": identifier,
        "legacy": {
            "id_str": identifier,
            "user_id_str": author,
            "created_at": "Tue Sep 22 06:00:00 +0000 2026",
            "full_text": "受控原文",
            "lang": "zh",
            "conversation_id_str": identifier,
            "favorite_count": 0,
            **fields,
        },
        "core": {
            "user_results": {
                "result": {
                    "__typename": "User",
                    "rest_id": author,
                    "legacy": {
                        "id_str": author,
                        "screen_name": "controlled",
                        "name": "Controlled",
                        "created_at": "Tue Sep 22 06:00:00 +0000 2020",
                    },
                }
            }
        },
    }


def _timeline(*tweets: dict, cursor: str | None = None) -> dict:
    entries = [
        {
            "entryId": f"tweet-{tweet['rest_id']}",
            "content": {"itemContent": {"tweet_results": {"result": tweet}}},
        }
        for tweet in tweets
    ]
    if cursor is not None:
        entries.append(
            {"entryId": "cursor-bottom", "content": {"cursorType": "Bottom", "value": cursor}}
        )
    return {"data": {"timeline": {"instructions": [{"entries": entries}]}}}


def _adapter(
    handler: Callable[[httpx.Request], httpx.Response], **options: object
) -> XTwscrapeAdapter:
    arguments = {
        "session": XSession(
            auth_token=SecretStr("private-token"), csrf_token=SecretStr("private-csrf")
        ),
        "transport": httpx.MockTransport(handler),
        "before_request": lambda _: True,
        "transaction_id": lambda _method, _path: "controlled-transaction",
    }
    return XTwscrapeAdapter(**{**arguments, **options})


def _search(**options: object) -> SearchRequest:
    return SearchRequest(source_key="x", query="known topic", page_size=20, **options)


def test_transport_logs_do_not_expose_query_or_session(
    caplog: pytest.LogCaptureFixture, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(logging, "basicConfig", lambda **_: None)
    for name in ("httpx", "httpcore"):
        monkeypatch.setattr(logging.getLogger(name), "level", logging.NOTSET)
    configure_logging("DEBUG")
    with caplog.at_level(logging.DEBUG):
        page = _adapter(lambda _: httpx.Response(200, json=_timeline())).fetch_page(_search())
        logging.getLogger("httpcore.http11").debug("headers=%s", "private-token")
    assert page.state is SourcePageState.EMPTY
    assert "rawQuery" not in caplog.text
    assert "private-token" not in caplog.text


def test_repost_preserves_original_identity_text_and_target() -> None:
    post = _tweet("100", full_text="原始转帖文本")
    post["legacy"]["retweeted_status_result"] = {"result": _tweet("200", author="99")}
    page = _adapter(lambda _: httpx.Response(200, json=_timeline(post))).fetch_page(_search())
    assert len(page.items) == 1
    assert page.items[0].external_id == "100"
    assert page.items[0].author_external_id == "42"
    assert page.items[0].text == "原始转帖文本"
    assert page.items[0].repost_external_id == "200"


def test_search_preserves_large_ids_missing_counts_and_raw_text() -> None:
    requests = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json=_timeline(_tweet()))

    page = _adapter(handler).fetch_page(_search())

    assert page.state is SourcePageState.COMPLETE
    assert page.items[0].external_id == "9007199254740993"
    assert page.items[0].like_count == 0
    assert page.items[0].comment_count is None
    assert page.items[0].text == "受控原文"
    assert page.request_count == 1
    assert json.loads(requests[0].url.params["variables"])["product"] == "Latest"


@pytest.mark.parametrize(
    ("status", "reason"),
    [
        (401, SourceStopReason.AUTHENTICATION_REQUIRED),
        (403, SourceStopReason.ACCESS_DENIED),
        (429, SourceStopReason.RATE_LIMITED),
        (500, SourceStopReason.UPSTREAM_ERROR),
    ],
)
def test_failed_request_is_counted_and_never_retried(status: int, reason: SourceStopReason) -> None:
    calls = []
    adapter = _adapter(lambda request: calls.append(request) or httpx.Response(status))
    page = adapter.fetch_page(_search())
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is reason
    assert page.request_count == len(calls) == 1


def test_valid_empty_is_distinct_from_missing_timeline_and_parse_failure() -> None:
    empty = _adapter(lambda _: httpx.Response(200, json=_timeline())).fetch_page(_search())
    malformed = _adapter(lambda _: httpx.Response(200, json={"data": {}})).fetch_page(_search())
    broken = _tweet(created_at="invalid")
    failed = _adapter(lambda _: httpx.Response(200, json=_timeline(broken))).fetch_page(_search())
    assert empty.state is SourcePageState.EMPTY
    assert malformed.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert failed.stop_reason is SourceStopReason.PROTOCOL_ERROR
    invalid_json = _adapter(lambda _: httpx.Response(200, text="not-json")).fetch_page(_search())
    assert invalid_json.stop_reason is SourceStopReason.PROTOCOL_ERROR
    missing_text = _tweet()
    del missing_text["legacy"]["full_text"]
    page = _adapter(lambda _: httpx.Response(200, json=_timeline(missing_text))).fetch_page(
        _search()
    )
    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR


def test_author_mismatch_is_rejected_and_root_post_is_not_a_comment() -> None:
    valid = _adapter(lambda _: httpx.Response(200, json=_timeline(_tweet()))).fetch_page(
        AuthorPostsRequest(source_key="x", author_external_id="42", page_size=20)
    )
    assert valid.state is SourcePageState.COMPLETE
    assert valid.items[0].author_external_id == "42"
    adapter = _adapter(lambda _: httpx.Response(200, json=_timeline(_tweet())))
    page = adapter.fetch_page(
        AuthorPostsRequest(source_key="x", author_external_id="43", page_size=20)
    )
    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    root = _tweet("100")
    direct = _tweet("101", conversation_id_str="100", in_reply_to_status_id_str="100")
    nested = _tweet("102", conversation_id_str="100", in_reply_to_status_id_str="101")
    adapter = _adapter(lambda _: httpx.Response(200, json=_timeline(root, direct, nested)))
    page = adapter.fetch_page(CommentsRequest(source_key="x", post_external_id="100", page_size=20))
    assert [item.external_id for item in page.items] == ["101", "102"]
    assert [item.parent_comment_external_id for item in page.items] == [None, "101"]


def test_request_budget_stops_before_next_network_call() -> None:
    calls = []
    adapter = _adapter(
        lambda req: calls.append(req) or httpx.Response(200, json=_timeline(_tweet(), cursor="A")),
        max_requests=1,
    )
    first = adapter.fetch_page(_search())
    second = adapter.fetch_page(_search(page_token="A"))
    assert first.state is SourcePageState.MORE
    assert second.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert second.request_count == 0
    assert len(calls) == 1


def test_cycle_across_pages_is_not_reported_complete() -> None:
    cursors = iter(["A", "B", "A"])
    adapter = _adapter(
        lambda _: httpx.Response(200, json=_timeline(_tweet(), cursor=next(cursors)))
    )
    adapter.fetch_page(_search())
    adapter.fetch_page(_search(page_token="A"))
    page = adapter.fetch_page(_search(page_token="B"))
    assert page.stop_reason is SourceStopReason.CURSOR_LOOP
    assert page.watermark is None


def test_top_and_nested_reply_inputs_keep_source_semantics() -> None:
    calls = []
    adapter = _adapter(lambda req: calls.append(req) or httpx.Response(200, json=_timeline()))
    adapter.fetch_page(_search(sort=SourceSort.TOP))
    assert json.loads(calls[0].url.params["variables"])["product"] == "Top"
    tweets = [
        _tweet("101", conversation_id_str="100", in_reply_to_status_id_str="100"),
        _tweet("102", conversation_id_str="100", in_reply_to_status_id_str="101"),
    ]
    adapter = _adapter(lambda _: httpx.Response(200, json=_timeline(*tweets)))
    page = adapter.fetch_page(
        RepliesRequest(
            source_key="x", post_external_id="100", comment_external_id="101", page_size=20
        )
    )
    assert [item.external_id for item in page.items] == ["102"]
    assert page.items[0].post_external_id == "100"


@pytest.mark.parametrize(
    "options,reason",
    [
        ({"session": None}, SourceStopReason.AUTHENTICATION_REQUIRED),
        ({"cancelled": lambda: True}, SourceStopReason.CANCELLED),
        ({"before_request": lambda _: False}, SourceStopReason.BUDGET_EXHAUSTED),
    ],
)
def test_rejected_preflight_never_sends_requests(options: dict, reason: SourceStopReason) -> None:
    calls = []
    adapter = _adapter(
        lambda req: calls.append(req) or httpx.Response(200, json=_timeline()), **options
    )
    page = adapter.fetch_page(_search())
    assert page.stop_reason is reason
    assert page.request_count == 0
    assert not calls


@pytest.mark.parametrize(
    "code,reason",
    [
        (32, SourceStopReason.AUTHENTICATION_REQUIRED),
        (88, SourceStopReason.RATE_LIMITED),
        (326, SourceStopReason.ACCESS_DENIED),
        (999, SourceStopReason.PROTOCOL_ERROR),
    ],
)
def test_graphql_errors_with_http_200_never_become_empty_success(
    code: int, reason: SourceStopReason
) -> None:
    page = _adapter(
        lambda _: httpx.Response(
            200, json={"errors": [{"code": code, "message": "private response"}]}
        )
    ).fetch_page(_search())
    assert page.stop_reason is reason
    assert "private response" not in page.model_dump_json()


def test_terminal_rate_limit_does_not_resume_or_rotate_session() -> None:
    calls = []
    adapter = _adapter(
        lambda req: (
            calls.append(req) or httpx.Response(429, headers={"x-rate-limit-reset": "2000000000"})
        )
    )
    first = adapter.fetch_page(_search())
    second = adapter.fetch_page(_search())
    assert first.retry_at.timestamp() == 2000000000
    assert second.stop_reason is SourceStopReason.RATE_LIMITED
    assert second.request_count == 0
    assert len(calls) == 1


def test_consecutive_empty_pages_and_record_limit_are_partial_boundaries() -> None:
    cursors = iter(["A", "B", "C"])
    adapter = _adapter(lambda _: httpx.Response(200, json=_timeline(cursor=next(cursors))))
    assert adapter.fetch_page(_search()).state is SourcePageState.MORE
    assert adapter.fetch_page(_search(page_token="A")).state is SourcePageState.MORE
    assert (
        adapter.fetch_page(_search(page_token="B")).stop_reason is SourceStopReason.PROTOCOL_ERROR
    )
    adapter = _adapter(lambda _: httpx.Response(200, json=_timeline(_tweet("1"), _tweet("2"))))
    page = adapter.fetch_page(SearchRequest(source_key="x", query="topic", page_size=1))
    assert page.state is SourcePageState.PARTIAL
    assert page.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert len(page.items) == 1
    assert page.watermark is None


def test_quote_target_is_not_an_extra_search_hit() -> None:
    post = _tweet("100", quoted_status_id_str="200")
    post["quoted_status_result"] = {"result": _tweet("200", author="99")}
    page = _adapter(lambda _: httpx.Response(200, json=_timeline(post))).fetch_page(_search())
    assert [item.external_id for item in page.items] == ["100"]
    assert page.items[0].quote_external_id == "200"


def test_bad_payload_does_not_dump_or_expose_source_data(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture
) -> None:
    import twscrape.models

    def forbidden_dump(*_: object) -> None:
        pytest.fail("SDK attempted to dump private response")

    monkeypatch.setattr(twscrape.models, "_write_dump", forbidden_dump)
    broken = _tweet(created_at="private-invalid-date")
    page = _adapter(lambda _: httpx.Response(200, json=_timeline(broken))).fetch_page(_search())
    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert "private" not in page.model_dump_json()
    captured = capsys.readouterr()
    assert "private" not in captured.out + captured.err
    assert os.environ["TWS_TELEMETRY"] == "0"


class _WaitingTransport(httpx.AsyncBaseTransport):
    def __init__(self) -> None:
        self.started = False
        self.cancelled = False
        self.closed = False

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        self.started = True
        try:
            await asyncio.Event().wait()
        except asyncio.CancelledError:
            self.cancelled = True
            raise
        raise AssertionError("unreachable")

    async def aclose(self) -> None:
        self.closed = True


@pytest.mark.parametrize("cancel", [True, False])
def test_inflight_cancel_and_deadline_release_transport(cancel: bool) -> None:
    transport = _WaitingTransport()
    adapter = _adapter(
        lambda _: httpx.Response(200),
        transport=transport,
        max_seconds=0.15,
        cancelled=lambda: cancel and transport.started,
    )
    page = adapter.fetch_page(_search())
    assert page.stop_reason is (
        SourceStopReason.CANCELLED if cancel else SourceStopReason.BUDGET_EXHAUSTED
    )
    assert page.request_count == 1
    assert transport.closed and transport.cancelled


def test_initialization_requests_share_budget_and_never_send_cookies_to_assets(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from sources.adapters import x_twscrape

    async def keys(_soup: object, client: object) -> tuple[list[int], str]:
        await client.get("https://abs.twimg.com/x-web/controlled.js")
        return [1] * 32, "controlled"

    monkeypatch.setattr(x_twscrape, "load_keys", keys)
    calls = []
    reservations = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json=_timeline())

    adapter = _adapter(
        handler,
        transaction_id=None,
        before_request=lambda index: reservations.append(index) or True,
    )
    page = adapter.fetch_page(_search())
    assert page.state is SourcePageState.EMPTY
    assert page.request_count == 3
    assert reservations == [1, 2, 3]
    assert "cookie" in calls[0].headers
    assert "cookie" not in calls[1].headers
    assert "authorization" not in calls[1].headers
    assert "x-csrf-token" not in calls[1].headers
    adapter = _adapter(handler, transaction_id=None, max_requests=1)
    page = adapter.fetch_page(_search())
    assert page.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert page.request_count == 1


def test_redirects_are_not_followed_and_network_failure_is_sanitized() -> None:
    page = _adapter(
        lambda _: httpx.Response(302, headers={"location": "http://127.0.0.1/private"})
    ).fetch_page(_search())
    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert page.request_count == 1

    def fail(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadTimeout("private-token and raw query", request=request)

    page = _adapter(fail).fetch_page(_search())
    assert page.stop_reason is SourceStopReason.UPSTREAM_ERROR
    assert "private" not in page.model_dump_json()
