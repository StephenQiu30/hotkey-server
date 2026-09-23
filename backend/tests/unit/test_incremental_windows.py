import hashlib
from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from pydantic import ValidationError

from jobs.schemas import CoverageWindowInput
from sources.contracts import SourceCapability, SourceSort

_TARGET_HASH = hashlib.sha256(b"controlled-target").digest()


def _window(**changes: object) -> dict[str, object]:
    end = datetime(2026, 9, 23, 2, tzinfo=UTC)
    return {
        "owner_id": uuid4(),
        "source_key": "x",
        "capability": SourceCapability.SEARCH,
        "target_hash": _TARGET_HASH,
        "sort_key": SourceSort.LATEST,
        "rule_version": 1,
        "starts_at": end - timedelta(hours=1),
        "ends_at": end,
        **changes,
    }


@pytest.mark.parametrize(
    "changes",
    [
        {"starts_at": datetime(2026, 9, 23, 1)},
        {"starts_at": datetime(2026, 9, 23, 2, tzinfo=UTC)},
        {"target_hash": b"short"},
        {"rule_version": 0},
        {"capability": SourceCapability.PAGE_CONTENT},
    ],
)
def test_window_rejects_ambiguous_or_invalid_scope(changes: dict[str, object]) -> None:
    with pytest.raises(ValidationError):
        CoverageWindowInput.model_validate(_window(**changes))


def test_window_accepts_explicit_utc_half_open_range() -> None:
    window = CoverageWindowInput.model_validate(_window())

    assert window.starts_at < window.ends_at
    assert window.starts_at.utcoffset() == timedelta(0)


def test_cursor_cycle_restarts_from_window_without_persisting_raw_tokens() -> None:
    from jobs.cursor import advance_cursor_page, plan_cursor_request
    from sources.contracts import SourcePageState, SourceStopReason

    window = CoverageWindowInput.model_validate(_window())
    checkpoint: dict[str, str | int | bool | None] = {}
    token: str | None = None
    for next_token in ("secret-A", "secret-B", "secret-A"):
        request = plan_cursor_request(
            window=window,
            checkpoint=checkpoint,
            live_token=token,
            max_pages=8,
            max_rescans=1,
        )
        page = advance_cursor_page(request, state=SourcePageState.MORE, next_token=next_token)
        checkpoint, token = page.checkpoint, page.next_token

    assert page.state is SourcePageState.PARTIAL
    assert page.stop_reason is SourceStopReason.CURSOR_LOOP
    assert page.next_token is None
    assert "secret-" not in str(checkpoint)
    restart = plan_cursor_request(
        window=window,
        checkpoint=checkpoint,
        live_token=None,
        max_pages=8,
        max_rescans=1,
    )
    assert restart.restarted is True
    assert restart.token is None


def test_expired_cursor_and_crash_rescan_are_bounded() -> None:
    from jobs.cursor import CursorBudgetExhaustedError, advance_cursor_page, plan_cursor_request
    from sources.contracts import SourcePageState, SourceStopReason

    window = CoverageWindowInput.model_validate(_window())
    first = plan_cursor_request(
        window=window, checkpoint={}, live_token=None, max_pages=3, max_rescans=1
    )
    page = advance_cursor_page(first, state=SourcePageState.MORE, next_token="private-cursor")
    assert "private-cursor" not in repr(page)
    with pytest.raises(ValueError, match="limits cannot change"):
        plan_cursor_request(
            window=window,
            checkpoint=page.checkpoint,
            live_token="private-cursor",
            max_pages=32,
            max_rescans=1,
        )
    continued = plan_cursor_request(
        window=window,
        checkpoint=page.checkpoint,
        live_token="private-cursor",
        max_pages=3,
        max_rescans=1,
    )
    assert "private-cursor" not in repr(continued)
    recovered = plan_cursor_request(
        window=window,
        checkpoint=page.checkpoint,
        live_token=None,
        max_pages=3,
        max_rescans=1,
    )
    assert recovered.restarted is True
    assert "private-cursor" not in repr(recovered)
    expired = advance_cursor_page(
        recovered,
        state=SourcePageState.PARTIAL,
        stop_reason=SourceStopReason.CURSOR_EXPIRED,
    )
    assert expired.stop_reason is SourceStopReason.CURSOR_EXPIRED
    with pytest.raises(CursorBudgetExhaustedError):
        plan_cursor_request(
            window=window,
            checkpoint=expired.checkpoint,
            live_token=None,
            max_pages=3,
            max_rescans=1,
        )
    assert "private-cursor" not in str(expired.checkpoint)


def test_cursor_page_budget_stops_before_an_unknown_tail() -> None:
    from jobs.cursor import CursorBudgetExhaustedError, advance_cursor_page, plan_cursor_request
    from sources.contracts import SourcePageState, SourceStopReason

    window = CoverageWindowInput.model_validate(_window())
    request = plan_cursor_request(
        window=window, checkpoint={}, live_token=None, max_pages=1, max_rescans=0
    )
    page = advance_cursor_page(request, state=SourcePageState.MORE, next_token="secret-tail")
    assert page.state is SourcePageState.PARTIAL
    assert page.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    with pytest.raises(CursorBudgetExhaustedError):
        plan_cursor_request(
            window=window,
            checkpoint=page.checkpoint,
            live_token=None,
            max_pages=1,
            max_rescans=0,
        )

    terminal = advance_cursor_page(
        request,
        state=SourcePageState.COMPLETE,
        stop_reason=SourceStopReason.END_OF_RESULTS,
    )
    assert terminal.stop_reason is None


def test_rate_limited_search_can_only_rescan_within_remaining_page_budget() -> None:
    from jobs.cursor import CursorBudgetExhaustedError, advance_cursor_page, plan_cursor_request
    from sources.contracts import SourcePageState, SourceStopReason

    window = CoverageWindowInput.model_validate(_window())
    first = plan_cursor_request(
        window=window, checkpoint={}, live_token=None, max_pages=3, max_rescans=1
    )
    limited = advance_cursor_page(
        first,
        state=SourcePageState.STOPPED,
        stop_reason=SourceStopReason.RATE_LIMITED,
    )
    retry = plan_cursor_request(
        window=window,
        checkpoint=limited.checkpoint,
        live_token=None,
        max_pages=3,
        max_rescans=1,
    )
    assert retry.restarted and retry.pages == 1 and retry.rescans == 1
    limited_again = advance_cursor_page(
        retry,
        state=SourcePageState.STOPPED,
        stop_reason=SourceStopReason.RATE_LIMITED,
    )
    with pytest.raises(CursorBudgetExhaustedError):
        plan_cursor_request(
            window=window,
            checkpoint=limited_again.checkpoint,
            live_token=None,
            max_pages=3,
            max_rescans=1,
        )
