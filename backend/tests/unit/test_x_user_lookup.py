from __future__ import annotations

from collections.abc import Callable

import httpx
import pytest
from pydantic import SecretStr

from sources.adapters.x_user_lookup import XUserLookupAdapter, parse_x_handle
from sources.contracts import SourceStopReason


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        ("@XDevelopers", "XDevelopers"),
        ("XDevelopers", "XDevelopers"),
        ("https://x.com/XDevelopers", "XDevelopers"),
        ("https://www.x.com/XDevelopers/", "XDevelopers"),
    ],
)
def test_profile_input_extracts_only_official_handle(value: str, expected: str) -> None:
    assert parse_x_handle(value) == expected


@pytest.mark.parametrize(
    "value",
    [
        "",
        "@",
        "user name",
        "@a/b",
        "https://x.com/i/123",
        "https://x.com/XDevelopers/status/1",
        "http://x.com/XDevelopers",
        "https://x.com.evil.test/XDevelopers",
        "https://x.com:444/XDevelopers",
        "https://user@x.com/XDevelopers",
        "https://x.com/XDevelopers?x=1",
        "https://x.com/XDevelopers#bio",
        "https://x.com/abcdefghijklmnop",
    ],
)
def test_profile_input_rejects_ambiguous_or_unsafe_values(value: str) -> None:
    with pytest.raises(ValueError):
        parse_x_handle(value)


def _adapter(
    respond: httpx.BaseTransport,
    *,
    authorize_request: Callable[[int, int], bool] = lambda attempt, max_users: True,
    settle_request: Callable[[int, int | None], None] = lambda attempt, actual_users: None,
) -> XUserLookupAdapter:
    return XUserLookupAdapter(
        token=SecretStr("fixture-token"),
        transport=respond,
        authorize_request=authorize_request,
        settle_request=settle_request,
    )


def test_lookup_requires_mock_transport() -> None:
    with pytest.raises(ValueError, match="offline"):
        _adapter(httpx.HTTPTransport())


def test_lookup_uses_official_endpoint_and_stable_id_only() -> None:
    calls: list[httpx.Request] = []
    approvals: list[tuple[int, int]] = []
    settlements: list[tuple[int, int | None]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(
            200,
            json={
                "data": {
                    "id": "2244994945",
                    "username": "XDevelopers",
                    "name": "Same display name",
                    "public_metrics": {"followers_count": 999999},
                }
            },
        )

    result = _adapter(
        httpx.MockTransport(respond),
        authorize_request=lambda attempt, max_users: approvals.append((attempt, max_users)) or True,
        settle_request=lambda attempt, count: settlements.append((attempt, count)),
    ).lookup("https://x.com/XDevelopers")

    assert result.stop_reason is None
    assert result.request_count == 1
    assert result.user is not None
    assert (result.user.source_key, result.user.external_id) == ("x", "2244994945")
    assert (result.user.username, result.user.display_name) == (
        "XDevelopers",
        "Same display name",
    )
    assert not hasattr(result.user, "followers_count")
    assert len(calls) == 1
    assert calls[0].method == "GET"
    assert str(calls[0].url) == "https://api.x.com/2/users/by/username/XDevelopers"
    assert calls[0].headers["Authorization"] == "Bearer fixture-token"
    assert approvals == [(1, 1)]
    assert settlements == [(1, 1)]


def test_budget_denial_never_sends_request() -> None:
    calls: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        raise AssertionError("network must not be touched")

    result = _adapter(
        httpx.MockTransport(respond),
        authorize_request=lambda attempt, max_users: False,
    ).lookup("@XDevelopers")
    assert result.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert result.request_count == 0
    assert result.user is None
    assert calls == []


@pytest.mark.parametrize(
    ("status", "reason"),
    [
        (302, SourceStopReason.PROTOCOL_ERROR),
        (401, SourceStopReason.AUTHENTICATION_REQUIRED),
        (403, SourceStopReason.ACCESS_DENIED),
        (404, SourceStopReason.NOT_FOUND),
        (429, SourceStopReason.RATE_LIMITED),
        (500, SourceStopReason.UPSTREAM_ERROR),
    ],
)
def test_lookup_failures_are_not_identity_confirmation(
    status: int, reason: SourceStopReason
) -> None:
    settlements: list[tuple[int, int | None]] = []
    result = _adapter(
        httpx.MockTransport(lambda request: httpx.Response(status)),
        settle_request=lambda attempt, count: settlements.append((attempt, count)),
    ).lookup("@XDevelopers")
    assert result.stop_reason is reason
    assert result.user is None
    assert result.request_count == 1
    assert settlements == [(1, None)]


@pytest.mark.parametrize(
    "payload",
    [
        {},
        {"data": []},
        {"data": {"id": "123", "username": "XDevelopers"}, "errors": [{"title": "bad"}]},
        {"data": {"id": "not-id", "username": "XDevelopers"}},
        {"data": {"id": "123", "username": "bad/handle"}},
        {"data": {"id": "123", "username": "other", "name": "Other"}},
        {"data": {"id": "123", "username": "XDevelopers", "name": 123}},
    ],
)
def test_invalid_identity_response_is_conservative(payload: object) -> None:
    settlements: list[tuple[int, int | None]] = []
    result = _adapter(
        httpx.MockTransport(lambda request: httpx.Response(200, json=payload)),
        settle_request=lambda attempt, count: settlements.append((attempt, count)),
    ).lookup("@XDevelopers")
    assert result.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert result.user is None
    assert settlements == [(1, None)]


def test_two_users_with_same_display_name_remain_distinct_ids() -> None:
    ids = ("123", "456")
    results = [
        _adapter(
            httpx.MockTransport(
                lambda request, user_id=user_id: httpx.Response(
                    200,
                    json={"data": {"id": user_id, "username": "first", "name": "Same"}},
                )
            )
        ).lookup("@first")
        for user_id in ids
    ]
    assert [result.user.external_id for result in results if result.user is not None] == list(ids)


def test_changed_username_does_not_change_stable_identity() -> None:
    users = ("original", "renamed")
    results = [
        _adapter(
            httpx.MockTransport(
                lambda request, username=username: httpx.Response(
                    200,
                    json={"data": {"id": "123", "username": username, "name": "Same"}},
                )
            )
        ).lookup(f"@{username}")
        for username in users
    ]
    assert [result.user.external_id for result in results if result.user is not None] == [
        "123",
        "123",
    ]


def test_transport_error_settles_unknown_usage_without_confirming_identity() -> None:
    settlements: list[tuple[int, int | None]] = []

    def fail(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("offline failure", request=request)

    result = _adapter(
        httpx.MockTransport(fail),
        settle_request=lambda attempt, count: settlements.append((attempt, count)),
    ).lookup("@XDevelopers")
    assert result.stop_reason is SourceStopReason.UPSTREAM_ERROR
    assert result.request_count == 1
    assert result.user is None
    assert settlements == [(1, None)]
