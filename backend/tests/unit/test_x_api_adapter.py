from __future__ import annotations

import logging
from collections.abc import Callable

import httpx
import pytest
from pydantic import SecretStr

from sources.adapters.x_api import XApiAdapter
from sources.contracts import SearchRequest, SourcePageState, SourceSort, SourceStopReason


def _request(**overrides: object) -> SearchRequest:
    values: dict[str, object] = {"source_key": "x", "query": "known topic", "page_size": 10}
    return SearchRequest(**(values | overrides))


def _adapter(
    handler: Callable[[httpx.Request], httpx.Response],
    **overrides: object,
) -> XApiAdapter:
    values: dict[str, object] = {
        "token": SecretStr("test-secret-token"),
        "transport": httpx.MockTransport(handler),
        "authorize_request": lambda _attempt, _max_posts: True,
        "settle_request": lambda _attempt, _posts: None,
    }
    return XApiAdapter(**(values | overrides))


def test_recent_search_maps_sort_fields_and_page_token() -> None:
    calls: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(
            200,
            json={
                "data": [
                    {
                        "id": "9007199254740993",
                        "text": "受控原文",
                        "author_id": "42",
                        "created_at": "2026-09-22T06:00:00Z",
                        "lang": "zh",
                        "conversation_id": "9007199254740993",
                        "public_metrics": {"like_count": 0, "reply_count": 2},
                    }
                ],
                "meta": (
                    {"result_count": 1}
                    if request.url.params.get("next_token")
                    else {"result_count": 1, "next_token": "ABCD1234"}
                ),
            },
        )

    adapter = _adapter(respond)
    page = adapter.fetch_page(_request(sort=SourceSort.TOP))
    next_page = adapter.fetch_page(_request(sort=SourceSort.TOP, page_token="ABCD1234"))

    assert page.state is SourcePageState.MORE
    assert page.next_page_token == "ABCD1234"
    assert page.request_count == 1
    assert next_page.state is SourcePageState.COMPLETE
    assert next_page.request_count == 1
    post = page.items[0]
    assert post.external_id == "9007199254740993"
    assert post.author_external_id == "42"
    assert post.language == "zh"
    assert post.like_count == 0
    assert post.comment_count == 2
    assert post.repost_count is None
    assert post.canonical_url == "https://x.com/i/web/status/9007199254740993"
    assert calls[0].url.host == "api.x.com"
    assert calls[0].url.path == "/2/tweets/search/recent"
    assert calls[0].url.params["sort_order"] == "relevancy"
    assert calls[0].url.params["max_results"] == "10"
    assert "expansions" not in calls[0].url.params
    assert calls[1].url.params["next_token"] == "ABCD1234"
    assert calls[0].headers["authorization"] == "Bearer test-secret-token"


def test_recent_search_valid_empty_is_not_a_failure() -> None:
    calls: list[httpx.Request] = []
    settlements: list[tuple[int, int | None]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"meta": {"result_count": 0}})

    page = _adapter(
        respond,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())
    assert page.state is SourcePageState.EMPTY
    assert page.stop_reason is SourceStopReason.SOURCE_EMPTY
    assert page.request_count == 1
    assert calls[0].url.params["sort_order"] == "recency"
    assert settlements == [(1, 0)]


@pytest.mark.parametrize(
    ("status", "reason"),
    [
        (401, SourceStopReason.AUTHENTICATION_REQUIRED),
        (403, SourceStopReason.ACCESS_DENIED),
        (429, SourceStopReason.RATE_LIMITED),
        (500, SourceStopReason.UPSTREAM_ERROR),
    ],
)
def test_recent_search_classifies_http_failure(status: int, reason: SourceStopReason) -> None:
    page = _adapter(lambda _: httpx.Response(status, json={"detail": "private"})).fetch_page(
        _request()
    )
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is reason
    assert page.items == ()
    assert page.request_count == 1


@pytest.mark.parametrize(
    "payload",
    [
        {},
        {"data": [], "meta": {"result_count": 1}},
        {"data": [{"id": "1", "text": "missing author"}], "meta": {"result_count": 1}},
        {"errors": [{"detail": "private"}], "meta": {"result_count": 0}},
    ],
)
def test_recent_search_rejects_malformed_success(payload: dict[str, object]) -> None:
    page = _adapter(lambda _: httpx.Response(200, json=payload)).fetch_page(_request())
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert page.request_count == 1


def test_budget_rejects_before_any_request() -> None:
    calls: list[httpx.Request] = []
    settlements: list[tuple[int, int | None]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"meta": {"result_count": 0}})

    page = _adapter(
        respond,
        authorize_request=lambda _attempt, _max_posts: False,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())
    assert page.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert page.request_count == 0
    assert calls == []
    assert settlements == []


def test_invalid_page_size_rejects_without_billable_request() -> None:
    calls: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"meta": {"result_count": 0}})

    page = _adapter(respond).fetch_page(_request(page_size=5))
    assert page.stop_reason is SourceStopReason.UNSUPPORTED
    assert page.request_count == 0
    assert calls == []


def test_offline_slice_rejects_network_transport() -> None:
    with pytest.raises(ValueError, match="mock transport"):
        XApiAdapter(
            token=SecretStr("test-secret-token"),
            transport=httpx.HTTPTransport(),
            authorize_request=lambda _attempt, _max_posts: True,
            settle_request=lambda _attempt, _posts: None,
        )


def test_each_page_authorizes_maximum_and_settles_valid_resource_count() -> None:
    authorizations: list[tuple[int, int]] = []
    settlements: list[tuple[int, int | None]] = []

    def respond(_: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "data": [{"id": "1", "author_id": "42", "text": "post"}],
                "meta": {"result_count": 1},
            },
        )

    page = _adapter(
        respond,
        authorize_request=lambda attempt, max_posts: (
            authorizations.append((attempt, max_posts)) or True
        ),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request(page_size=100))

    assert page.state is SourcePageState.COMPLETE
    assert authorizations == [(1, 100)]
    assert settlements == [(1, 1)]


@pytest.mark.parametrize("status", [401, 429, 500])
def test_failed_request_keeps_unknown_billable_count(status: int) -> None:
    settlements: list[tuple[int, int | None]] = []
    page = _adapter(
        lambda _: httpx.Response(status),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())

    assert page.state is SourcePageState.STOPPED
    assert settlements == [(1, None)]


def test_malformed_response_keeps_unknown_billable_count() -> None:
    settlements: list[tuple[int, int | None]] = []
    page = _adapter(
        lambda _: httpx.Response(200, json={"meta": {"result_count": 1}}),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())

    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert settlements == [(1, None)]


def test_unrequested_expanded_resources_cannot_be_undercounted() -> None:
    settlements: list[tuple[int, int | None]] = []
    page = _adapter(
        lambda _: httpx.Response(
            200,
            json={
                "meta": {"result_count": 0},
                "includes": {"users": [{"id": "42"}]},
            },
        ),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())

    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert settlements == [(1, None)]


def test_transport_failure_still_settles_unknown_count() -> None:
    settlements: list[tuple[int, int | None]] = []

    def fail(_: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("offline failure")

    page = _adapter(
        fail,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())

    assert page.stop_reason is SourceStopReason.UPSTREAM_ERROR
    assert settlements == [(1, None)]


def test_cancelled_response_still_settles_unknown_count() -> None:
    settlements: list[tuple[int, int | None]] = []
    sent = False

    def respond(_: httpx.Request) -> httpx.Response:
        nonlocal sent
        sent = True
        return httpx.Response(200, json={"meta": {"result_count": 0}})

    page = _adapter(
        respond,
        cancelled=lambda: sent,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())

    assert page.stop_reason is SourceStopReason.CANCELLED
    assert page.request_count == 1
    assert settlements == [(1, None)]


def test_completed_page_does_not_repeat_request() -> None:
    calls: list[httpx.Request] = []
    settlements: list[tuple[int, int | None]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"meta": {"result_count": 0}})

    adapter = _adapter(
        respond,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    )
    first = adapter.fetch_page(_request())
    repeated = adapter.fetch_page(_request())
    assert first.state is SourcePageState.EMPTY
    assert repeated.state is SourcePageState.EMPTY
    assert repeated.request_count == 0
    assert len(calls) == 1
    assert settlements == [(1, 0)]


def test_no_token_or_query_is_logged(caplog: pytest.LogCaptureFixture) -> None:
    with caplog.at_level(logging.DEBUG):
        page = _adapter(lambda _: httpx.Response(200, json={"meta": {"result_count": 0}}))
        result = page.fetch_page(_request())
    assert result.state is SourcePageState.EMPTY
    assert "test-secret-token" not in caplog.text
    assert "known topic" not in caplog.text
