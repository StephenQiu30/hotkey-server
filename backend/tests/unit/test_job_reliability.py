from __future__ import annotations

from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from pydantic import ValidationError

from jobs.execution import plan_catchup_windows, scheduled_operation_id
from jobs.schemas import JobAcceptanceInput, JobObservationContext
from jobs.services import fingerprint_request
from sources.contracts import SourceCapability


def _observation() -> JobObservationContext:
    return JobObservationContext(
        configuration_ref="monitor-config-1",
        configuration_version=1,
        source_key="x",
        source_capability=SourceCapability.SEARCH,
    )


def test_request_fingerprint_is_stable_for_equivalent_scope() -> None:
    operation_id = uuid4()
    first = JobAcceptanceInput(
        operation_id=operation_id,
        kind="monitor.collect",
        observation=_observation(),
        scope={"source_id": "account-1", "window": 7},
    )
    reordered = JobAcceptanceInput(
        operation_id=operation_id,
        kind="monitor.collect",
        observation=_observation(),
        scope={"window": 7, "source_id": "account-1"},
    )
    changed = JobAcceptanceInput(
        operation_id=operation_id,
        kind="monitor.collect",
        observation=_observation(),
        scope={"source_id": "account-1", "window": 8},
    )

    assert fingerprint_request(first) == fingerprint_request(reordered)
    assert fingerprint_request(first) != fingerprint_request(changed)


def test_acceptance_input_rejects_unbounded_or_unknown_scope() -> None:
    with pytest.raises(ValidationError):
        JobAcceptanceInput(
            operation_id=uuid4(),
            kind="monitor.collect",
            observation=_observation(),
            scope={f"key_{index}": index for index in range(33)},
        )

    with pytest.raises(ValidationError):
        JobAcceptanceInput(
            operation_id=uuid4(),
            kind="monitor.collect",
            observation=_observation(),
            scope={},
            owner_id=uuid4(),
        )


def test_catchup_plan_keeps_recent_windows_and_reports_skipped_work() -> None:
    start = datetime(2026, 9, 21, 0, tzinfo=UTC)

    plan = plan_catchup_windows(
        due_from=start,
        due_until=start + timedelta(hours=5),
        cadence=timedelta(hours=1),
        max_windows=3,
    )

    assert [(window.start, window.end) for window in plan.windows] == [
        (start + timedelta(hours=2), start + timedelta(hours=3)),
        (start + timedelta(hours=3), start + timedelta(hours=4)),
        (start + timedelta(hours=4), start + timedelta(hours=5)),
    ]
    assert plan.skipped_windows == 2


def test_schedule_operation_id_is_stable_per_owner_kind_key_and_window() -> None:
    owner_id = uuid4()
    start = datetime(2026, 9, 21, 0, tzinfo=UTC)
    plan = plan_catchup_windows(
        due_from=start,
        due_until=start + timedelta(hours=1),
        cadence=timedelta(hours=1),
        max_windows=3,
    )
    window = plan.windows[0]

    first = scheduled_operation_id(owner_id, "monitor.collect", "topic-1", window)
    repeated = scheduled_operation_id(owner_id, "monitor.collect", "topic-1", window)
    changed = scheduled_operation_id(owner_id, "monitor.collect", "topic-2", window)

    assert first == repeated
    assert first != changed
