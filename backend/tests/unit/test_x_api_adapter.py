from __future__ import annotations

import logging
from collections.abc import Callable
from datetime import UTC, datetime, timedelta, timezone
from types import SimpleNamespace

import httpx
import pytest
from pydantic import SecretStr

from sources.adapters import x_api
from sources.adapters.x_api import XApiAdapter
from sources.contracts import (
    AuthorPostsRequest,
    CommentsRequest,
    RepliesRequest,
    SearchRequest,
    SourceCapability,
    SourcePageState,
    SourceSort,
    SourceStopReason,
)


def _request(**overrides: object) -> SearchRequest:
    values: dict[str, object] = {"source_key": "x", "query": "known topic", "page_size": 10}
    return SearchRequest(**(values | overrides))


def _author_request(**overrides: object) -> AuthorPostsRequest:
    values: dict[str, object] = {
        "source_key": "x",
        "author_external_id": "2244994945",
        "page_size": 5,
    }
    return AuthorPostsRequest(**(values | overrides))


def _comments_request(**overrides: object) -> CommentsRequest:
    values: dict[str, object] = {"source_key": "x", "post_external_id": "100", "page_size": 10}
    return CommentsRequest(**(values | overrides))


def _replies_request(**overrides: object) -> RepliesRequest:
    values: dict[str, object] = {
        "source_key": "x",
        "comment_external_id": "101",
        "post_external_id": "100",
        "page_size": 10,
    }
    return RepliesRequest(**(values | overrides))


def test_direct_reply_search_maps_root_and_nested_parent() -> None:
    calls: list[httpx.Request] = []
    authorizations: list[tuple[int, int]] = []
    settlements: list[tuple[int, int | None]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        parent, child = ("100", "101") if len(calls) == 1 else ("101", "102")
        return httpx.Response(
            200,
            json={
                "data": [
                    {
                        "id": child,
                        "author_id": "42",
                        "conversation_id": "100",
                        "referenced_posts": [{"type": "replied_to", "id": parent}],
                        "text": "reply",
                    }
                ],
                "meta": {"result_count": 1},
            },
        )

    root = _adapter(
        respond,
        authorize_request=lambda attempt, posts: authorizations.append((attempt, posts)) or True,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_comments_request())
    nested = _adapter(respond).fetch_page(_replies_request())

    assert root.capability is SourceCapability.COMMENTS
    assert root.state is SourcePageState.COMPLETE
    assert root.items[0].external_id == "101"
    assert root.items[0].post_external_id == "100"
    assert root.items[0].parent_comment_external_id is None
    assert nested.capability is SourceCapability.REPLIES
    assert nested.items[0].external_id == "102"
    assert nested.items[0].post_external_id == "100"
    assert nested.items[0].parent_comment_external_id == "101"
    assert [call.url.params["query"] for call in calls] == [
        "in_reply_to_tweet_id:100",
        "in_reply_to_tweet_id:101",
    ]
    assert all(call.url.path == "/2/tweets/search/recent" for call in calls)
    assert authorizations == [(1, 10)]
    assert settlements == [(1, 1)]


def test_direct_replies_paginate_and_settle_valid_empty_page() -> None:
    calls: list[httpx.Request] = []
    settlements: list[tuple[int, int | None]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        if len(calls) == 1:
            return httpx.Response(
                200,
                json={
                    "data": [{"id": "101", "author_id": "42", "conversation_id": "100"}],
                    "meta": {"result_count": 1, "next_token": "ABCD"},
                },
            )
        return httpx.Response(200, json={"meta": {"result_count": 0}})

    adapter = _adapter(
        respond,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    )
    first = adapter.fetch_page(_comments_request())
    second = adapter.fetch_page(_comments_request(page_token="ABCD"))

    assert first.state is SourcePageState.MORE
    assert first.next_page_token == "ABCD"
    assert second.state is SourcePageState.EMPTY
    assert second.stop_reason is SourceStopReason.SOURCE_EMPTY
    assert calls[1].url.params["next_token"] == "ABCD"
    assert settlements == [(1, 1), (2, 0)]


def test_nested_reply_can_use_verified_response_root_when_request_omits_it() -> None:
    page = _adapter(
        lambda _request: httpx.Response(
            200,
            json={
                "data": [{"id": "102", "author_id": "42", "conversation_id": "100"}],
                "meta": {"result_count": 1},
            },
        )
    ).fetch_page(_replies_request(post_external_id=None))

    assert page.state is SourcePageState.COMPLETE
    assert page.items[0].post_external_id == "100"
    assert page.items[0].parent_comment_external_id == "101"


@pytest.mark.parametrize(
    ("source_request", "response_post"),
    [
        (
            _replies_request(post_external_id=None),
            {"id": "102", "author_id": "42", "text": "unknown root"},
        ),
        (
            _comments_request(),
            {
                "id": "101",
                "author_id": "42",
                "conversation_id": "100",
                "referenced_posts": [{"type": "replied_to", "id": "999"}],
            },
        ),
        (
            _replies_request(),
            {"id": "102", "author_id": "42", "conversation_id": "999"},
        ),
    ],
)
def test_direct_replies_reject_unknown_root_or_conflicting_relationship(
    source_request: CommentsRequest | RepliesRequest,
    response_post: dict[str, object],
) -> None:
    settlements: list[tuple[int, int | None]] = []
    page = _adapter(
        lambda _request: httpx.Response(
            200,
            json={"data": [response_post], "meta": {"result_count": 1}},
        ),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(source_request)

    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert page.items == ()
    assert page.request_count == 1
    assert settlements == [(1, None)]


@pytest.mark.parametrize(
    "response_post",
    [
        {"id": "12345678901234567890", "author_id": "42"},
        {"id": "100", "author_id": "bad/author"},
        {"id": "100", "author_id": "42", "conversation_id": "bad/root"},
        {
            "id": "100",
            "author_id": "42",
            "referenced_posts": [{"type": "replied_to", "id": "bad/parent"}],
        },
        {
            "id": "100",
            "author_id": "42",
            "referenced_posts": [{"type": "quoted", "id": 123}],
        },
        {
            "id": "100",
            "author_id": "42",
            "referenced_posts": [
                {"type": "quoted", "id": "101"},
                {"type": "quoted", "id": "102"},
            ],
        },
    ],
)
def test_invalid_x_identity_or_conflicting_reference_rejects_whole_page(
    response_post: dict[str, object],
) -> None:
    settlements: list[tuple[int, int | None]] = []
    page = _adapter(
        lambda _request: httpx.Response(
            200, json={"data": [response_post], "meta": {"result_count": 1}}
        ),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())

    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert page.items == ()
    assert page.request_count == 1
    assert settlements == [(1, None)]


@pytest.mark.parametrize(
    "source_request",
    [_comments_request(post_external_id="bad/path"), _replies_request(comment_external_id="bad")],
)
def test_direct_replies_reject_invalid_id_before_authorization(
    source_request: CommentsRequest | RepliesRequest,
) -> None:
    calls: list[httpx.Request] = []
    authorizations: list[tuple[int, int]] = []
    page = _adapter(
        lambda sent: calls.append(sent) or httpx.Response(200),
        authorize_request=lambda attempt, posts: authorizations.append((attempt, posts)) or True,
    ).fetch_page(source_request)

    assert page.stop_reason is SourceStopReason.UNSUPPORTED
    assert page.request_count == 0
    assert calls == []
    assert authorizations == []


def test_author_posts_uses_official_timeline_and_accounts_for_each_page() -> None:
    calls: list[httpx.Request] = []
    authorizations: list[tuple[int, int]] = []
    settlements: list[tuple[int, int | None]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        post: dict[str, object] = {"id": str(len(calls)), "text": "authored post"}
        if len(calls) == 2:
            post["author_id"] = "2244994945"
        return httpx.Response(
            200,
            json={
                "data": [post],
                "meta": {"result_count": 1, **({"next_token": "ABCD"} if len(calls) == 1 else {})},
            },
        )

    adapter = _adapter(
        respond,
        authorize_request=lambda attempt, posts: authorizations.append((attempt, posts)) or True,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    )
    first = adapter.fetch_page(_author_request())
    second = adapter.fetch_page(_author_request(page_token="ABCD"))

    assert adapter.capabilities == frozenset(
        {
            SourceCapability.SEARCH,
            SourceCapability.AUTHOR_POSTS,
            SourceCapability.COMMENTS,
            SourceCapability.REPLIES,
        }
    )
    assert first.state is SourcePageState.MORE
    assert first.next_page_token == "ABCD"
    assert first.adapter_version == "x-api-v2/user-posts"
    assert first.items[0].author_external_id == "2244994945"
    assert second.state is SourcePageState.COMPLETE
    assert second.items[0].author_external_id == "2244994945"
    assert all(call.url.path == "/2/users/2244994945/tweets" for call in calls)
    assert calls[0].url.params["max_results"] == "5"
    assert "expansions" not in calls[0].url.params
    assert calls[1].url.params["pagination_token"] == "ABCD"
    assert authorizations == [(1, 5), (2, 5)]
    assert settlements == [(1, 1), (2, 1)]


@pytest.mark.parametrize(
    "updates",
    [
        {"author_external_id": "not-a-numeric-id"},
        {"author_external_id": "12345678901234567890"},
        {"page_size": 4},
    ],
)
def test_author_posts_rejects_invalid_endpoint_input_before_authorization(
    updates: dict[str, object],
) -> None:
    calls: list[httpx.Request] = []
    authorizations: list[tuple[int, int]] = []
    page = _adapter(
        lambda request: calls.append(request) or httpx.Response(200),
        authorize_request=lambda attempt, posts: authorizations.append((attempt, posts)) or True,
    ).fetch_page(_author_request(**updates))

    assert page.stop_reason is SourceStopReason.UNSUPPORTED
    assert page.request_count == 0
    assert calls == []
    assert authorizations == []


def test_author_posts_rejects_mismatched_response_author_and_keeps_cost_unknown() -> None:
    settlements: list[tuple[int, int | None]] = []
    page = _adapter(
        lambda _request: httpx.Response(
            200,
            json={
                "data": [{"id": "1", "author_id": "42", "text": "wrong"}],
                "meta": {"result_count": 1},
            },
        ),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_author_request())

    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert page.items == ()
    assert page.request_count == 1
    assert settlements == [(1, None)]


def test_author_posts_rejects_unvalidated_non_string_author_id() -> None:
    calls: list[httpx.Request] = []
    page = _adapter(lambda request: calls.append(request) or httpx.Response(200)).fetch_page(
        _author_request().model_copy(update={"author_external_id": 42})
    )

    assert page.stop_reason is SourceStopReason.UNSUPPORTED
    assert page.request_count == 0
    assert calls == []


@pytest.mark.parametrize("page_size", [101, "5"])
def test_author_posts_rejects_unvalidated_page_size_before_authorization(
    page_size: object,
) -> None:
    calls: list[httpx.Request] = []
    authorizations: list[tuple[int, int]] = []
    page = _adapter(
        lambda request: calls.append(request) or httpx.Response(200),
        authorize_request=lambda attempt, posts: authorizations.append((attempt, posts)) or True,
    ).fetch_page(_author_request().model_copy(update={"page_size": page_size}))

    assert page.stop_reason is SourceStopReason.UNSUPPORTED
    assert page.request_count == 0
    assert calls == []
    assert authorizations == []


def test_author_posts_valid_empty_page_settles_zero() -> None:
    settlements: list[tuple[int, int | None]] = []
    page = _adapter(
        lambda _request: httpx.Response(200, json={"meta": {"result_count": 0}}),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_author_request())

    assert page.capability is SourceCapability.AUTHOR_POSTS
    assert page.state is SourcePageState.EMPTY
    assert page.stop_reason is SourceStopReason.SOURCE_EMPTY
    assert settlements == [(1, 0)]


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


def test_recent_search_uses_only_endpoint_declared_post_fields() -> None:
    endpoint_fields = {
        "id",
        "text",
        "created_at",
        "lang",
        "conversation_id",
        "public_metrics",
    }

    def respond(request: httpx.Request) -> httpx.Response:
        requested = set(request.url.params["post.fields"].split(","))
        if not requested <= endpoint_fields:
            return httpx.Response(400, json={"errors": [{"title": "invalid field"}]})
        return httpx.Response(
            200,
            json={
                "data": [{"id": "1", "author_id": "42", "text": "post"}],
                "meta": {"result_count": 1},
            },
        )

    page = _adapter(respond).fetch_page(_request())

    assert page.state is SourcePageState.COMPLETE
    assert page.items[0].author_external_id == "42"


def test_recent_search_sends_same_explicit_time_window_on_every_page() -> None:
    now = datetime(2026, 9, 23, 12, tzinfo=UTC)
    starts_at = now - timedelta(days=2)
    ends_at = now - timedelta(hours=1)
    calls: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(
            200,
            json={
                "data": [{"id": "1", "author_id": "42", "text": "post"}],
                "meta": {"result_count": 1, **({"next_token": "ABCD"} if len(calls) == 1 else {})},
            },
        )

    adapter = _adapter(respond, clock=lambda: now)
    first = adapter.fetch_page(_request(starts_at=starts_at, ends_at=ends_at))
    second = adapter.fetch_page(_request(starts_at=starts_at, ends_at=ends_at, page_token="ABCD"))

    assert first.state is SourcePageState.MORE
    assert second.state is SourcePageState.COMPLETE
    for call in calls:
        assert call.url.params["start_time"] == "2026-09-21T12:00:00Z"
        assert call.url.params["end_time"] == "2026-09-23T11:00:00Z"


@pytest.mark.parametrize("window_kind", ["older_than_recent", "future_end"])
def test_recent_search_rejects_outside_recent_window_before_authorization(
    window_kind: str,
) -> None:
    now = datetime(2026, 9, 23, 12, tzinfo=UTC)
    calls: list[httpx.Request] = []
    authorizations: list[tuple[int, int]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"meta": {"result_count": 0}})

    starts_at = (
        now - timedelta(days=7, seconds=1)
        if window_kind == "older_than_recent"
        else now - timedelta(hours=1)
    )
    ends_at = now + timedelta(seconds=1) if window_kind == "future_end" else now
    page = _adapter(
        respond,
        clock=lambda: now,
        authorize_request=lambda attempt, posts: authorizations.append((attempt, posts)) or True,
    ).fetch_page(_request(starts_at=starts_at, ends_at=ends_at))

    assert page.stop_reason is SourceStopReason.UNSUPPORTED
    assert page.request_count == 0
    assert authorizations == []
    assert calls == []


@pytest.mark.parametrize("window_kind", ["partial", "reversed", "non_utc"])
def test_recent_search_rejects_a_copied_invalid_window_before_authorization(
    window_kind: str,
) -> None:
    calls: list[httpx.Request] = []
    authorizations: list[tuple[int, int]] = []
    start = datetime(2026, 9, 22, 12, tzinfo=UTC)
    updates = {
        "partial": {"starts_at": start},
        "reversed": {"starts_at": start, "ends_at": start - timedelta(hours=1)},
        "non_utc": {
            "starts_at": start.astimezone(timezone(timedelta(hours=8))),
            "ends_at": start + timedelta(hours=1),
        },
    }
    malformed = _request().model_copy(update=updates[window_kind])

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"meta": {"result_count": 0}})

    page = _adapter(
        respond,
        clock=lambda: datetime(2026, 9, 23, 12, tzinfo=UTC),
        authorize_request=lambda attempt, posts: authorizations.append((attempt, posts)) or True,
    ).fetch_page(malformed)

    assert page.stop_reason is SourceStopReason.UNSUPPORTED
    assert page.request_count == 0
    assert authorizations == []
    assert calls == []


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


def test_missing_author_identity_stops_without_releasing_unknown_cost() -> None:
    settlements: list[tuple[int, int | None]] = []
    page = _adapter(
        lambda _: httpx.Response(
            200,
            json={"data": [{"id": "1", "text": "post"}], "meta": {"result_count": 1}},
        ),
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())

    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert page.items == ()
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


def test_response_after_collection_deadline_is_not_reported_as_complete(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    elapsed = [0.0]
    settlements: list[tuple[int, int | None]] = []
    monkeypatch.setattr(x_api, "time", SimpleNamespace(monotonic=lambda: elapsed[0]))

    def respond(_: httpx.Request) -> httpx.Response:
        elapsed[0] = 2.0
        return httpx.Response(
            200,
            json={
                "data": [{"id": "1", "author_id": "42", "text": "late post"}],
                "meta": {"result_count": 1},
            },
        )

    page = _adapter(
        respond,
        max_seconds=1.0,
        settle_request=lambda attempt, posts: settlements.append((attempt, posts)),
    ).fetch_page(_request())

    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert page.request_count == 1
    assert page.items == ()
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
