from __future__ import annotations

from uuid import uuid4

import pytest
from pydantic import ValidationError

from jobs.schemas import JobAcceptanceInput
from jobs.services import fingerprint_request


def test_request_fingerprint_is_stable_for_equivalent_scope() -> None:
    operation_id = uuid4()
    first = JobAcceptanceInput(
        operation_id=operation_id,
        kind="monitor.collect",
        scope={"source_id": "account-1", "window": 7},
    )
    reordered = JobAcceptanceInput(
        operation_id=operation_id,
        kind="monitor.collect",
        scope={"window": 7, "source_id": "account-1"},
    )
    changed = JobAcceptanceInput(
        operation_id=operation_id,
        kind="monitor.collect",
        scope={"source_id": "account-1", "window": 8},
    )

    assert fingerprint_request(first) == fingerprint_request(reordered)
    assert fingerprint_request(first) != fingerprint_request(changed)


def test_acceptance_input_rejects_unbounded_or_unknown_scope() -> None:
    with pytest.raises(ValidationError):
        JobAcceptanceInput(
            operation_id=uuid4(),
            kind="monitor.collect",
            scope={f"key_{index}": index for index in range(33)},
        )

    with pytest.raises(ValidationError):
        JobAcceptanceInput(
            operation_id=uuid4(),
            kind="monitor.collect",
            scope={},
            owner_id=uuid4(),
        )
