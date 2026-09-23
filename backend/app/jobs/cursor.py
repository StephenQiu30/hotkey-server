from __future__ import annotations

import hashlib
import json
from collections.abc import Mapping
from dataclasses import dataclass, field

from jobs.execution import CheckpointValue
from jobs.schemas import CoverageWindowInput
from sources.contracts import SourcePageState, SourceStopReason


class CursorBudgetExhaustedError(ValueError):
    """A bounded scan cannot issue another page request."""


@dataclass(frozen=True, slots=True)
class CursorPageRequest:
    window_digest: str
    pages: int
    rescans: int
    seen: tuple[str, ...]
    token: str | None = field(repr=False)
    restarted: bool
    max_pages: int
    max_rescans: int


@dataclass(frozen=True, slots=True)
class CursorPageProgress:
    checkpoint: dict[str, CheckpointValue]
    state: SourcePageState
    stop_reason: SourceStopReason | None
    next_token: str | None = field(repr=False)


def _digest(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()


def _window_digest(window: CoverageWindowInput) -> str:
    identity = json.dumps(
        [
            str(window.owner_id),
            window.source_key,
            window.capability.value,
            window.target_hash.hex(),
            window.sort_key.value,
            window.rule_version,
            window.starts_at.isoformat(),
            window.ends_at.isoformat(),
        ],
        separators=(",", ":"),
    )
    return _digest(identity)


def _read_count(checkpoint: Mapping[str, CheckpointValue], key: str) -> int:
    value = checkpoint.get(key)
    if type(value) is not int or value < 0:
        raise ValueError("cursor checkpoint contains an invalid counter")
    return value


def plan_cursor_request(
    *,
    window: CoverageWindowInput,
    checkpoint: Mapping[str, CheckpointValue],
    live_token: str | None,
    max_pages: int,
    max_rescans: int,
) -> CursorPageRequest:
    """Choose a page before network access; never recover a raw token from storage."""
    if not 1 <= max_pages <= 32 or not 0 <= max_rescans <= 3:
        raise ValueError("cursor limits must be bounded")
    digest = _window_digest(window)
    if checkpoint and checkpoint.get("cursor.window") == digest:
        pages = _read_count(checkpoint, "cursor.pages")
        rescans = _read_count(checkpoint, "cursor.rescans")
        if (
            _read_count(checkpoint, "cursor.max_pages") != max_pages
            or _read_count(checkpoint, "cursor.max_rescans") != max_rescans
        ):
            raise ValueError("cursor limits cannot change within a window")
        raw_seen = checkpoint.get("cursor.seen")
        expected = checkpoint.get("cursor.expected")
        restart = checkpoint.get("cursor.restart")
        done = checkpoint.get("cursor.done")
        if (
            not isinstance(raw_seen, str)
            or not isinstance(expected, str)
            or type(restart) is not bool
            or type(done) is not bool
            or len(raw_seen) > 64 * 32 + 31
            or (
                expected
                and (
                    len(expected) != 64 or any(char not in "0123456789abcdef" for char in expected)
                )
            )
        ):
            raise ValueError("cursor checkpoint is malformed")
        seen = tuple(raw_seen.split(",")) if raw_seen else ()
        if any(
            len(item) != 64 or any(char not in "0123456789abcdef" for char in item) for item in seen
        ) or len(seen) != len(set(seen)):
            raise ValueError("cursor checkpoint has invalid digests")
    elif checkpoint and "cursor.window" not in checkpoint:
        raise ValueError("cursor checkpoint belongs to another execution format")
    else:
        pages, rescans, seen, expected, restart, done = 0, 0, (), "", False, False

    if pages >= max_pages:
        raise CursorBudgetExhaustedError("cursor page budget is exhausted")
    if done:
        raise CursorBudgetExhaustedError("cursor scan is terminal")
    if restart or (expected and live_token is None):
        if rescans >= max_rescans:
            raise CursorBudgetExhaustedError("cursor rescan budget is exhausted")
        return CursorPageRequest(digest, pages, rescans + 1, (), None, True, max_pages, max_rescans)
    if expected:
        if live_token is None or _digest(live_token) != expected:
            raise ValueError("live cursor does not match the persisted page")
    elif live_token is not None:
        raise ValueError("cursor cannot skip the first page of a window")
    return CursorPageRequest(
        digest, pages, rescans, seen, live_token, False, max_pages, max_rescans
    )


def advance_cursor_page(
    request: CursorPageRequest,
    *,
    state: SourcePageState,
    next_token: str | None = None,
    stop_reason: SourceStopReason | None = None,
) -> CursorPageProgress:
    """Store only digests and counters; a crash conservatively restarts the window."""
    pages = request.pages + 1
    if state is SourcePageState.MORE:
        if not next_token or stop_reason is not None:
            raise ValueError("continuing page requires only a next cursor")
        next_digest = _digest(next_token)
        if next_digest in request.seen:
            state, stop_reason, next_token = (
                SourcePageState.PARTIAL,
                SourceStopReason.CURSOR_LOOP,
                None,
            )
        elif pages >= request.max_pages:
            state, stop_reason, next_token = (
                SourcePageState.PARTIAL,
                SourceStopReason.BUDGET_EXHAUSTED,
                None,
            )
        else:
            seen = (*request.seen, next_digest)
    elif (
        next_token is not None
        or (state in {SourcePageState.PARTIAL, SourcePageState.STOPPED} and stop_reason is None)
        or (
            state is SourcePageState.COMPLETE
            and stop_reason not in {None, SourceStopReason.END_OF_RESULTS}
        )
        or (
            state is SourcePageState.EMPTY
            and stop_reason not in {None, SourceStopReason.SOURCE_EMPTY}
        )
    ):
        raise ValueError("terminal page has invalid cursor or stop reason")
    if state in {SourcePageState.COMPLETE, SourcePageState.EMPTY}:
        stop_reason = None
    restart = stop_reason in {SourceStopReason.CURSOR_EXPIRED, SourceStopReason.CURSOR_LOOP}
    checkpoint: dict[str, CheckpointValue] = {
        "cursor.window": request.window_digest,
        "cursor.pages": pages,
        "cursor.rescans": request.rescans,
        "cursor.max_pages": request.max_pages,
        "cursor.max_rescans": request.max_rescans,
        "cursor.seen": ",".join(seen if state is SourcePageState.MORE else request.seen),
        "cursor.expected": _digest(next_token) if next_token is not None else "",
        "cursor.restart": restart,
        "cursor.done": state is not SourcePageState.MORE and not restart,
    }
    return CursorPageProgress(checkpoint, state, stop_reason, next_token)
