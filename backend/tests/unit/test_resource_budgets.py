from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import pytest
from pydantic import ValidationError

from jobs.execution import resource_attempt_id
from jobs.schemas import ComponentPolicyInput, CostClass, UsageKind


def test_resource_attempt_id_is_stable_for_replay_and_distinct_for_retry() -> None:
    operation_id = uuid4()

    first = resource_attempt_id(
        operation_id=operation_id,
        component_key="collector.http",
        stage="search.page",
        sequence=1,
    )

    assert first == resource_attempt_id(
        operation_id=operation_id,
        component_key="collector.http",
        stage="search.page",
        sequence=1,
    )
    assert first != resource_attempt_id(
        operation_id=operation_id,
        component_key="collector.http",
        stage="search.page",
        sequence=2,
    )


@pytest.mark.parametrize("sequence", [-1, 0])
def test_resource_attempt_id_rejects_non_positive_retry_sequences(sequence: int) -> None:
    with pytest.raises(ValueError, match="sequence"):
        resource_attempt_id(
            operation_id=uuid4(),
            component_key="collector.http",
            stage="search.page",
            sequence=sequence,
        )


def test_component_policy_rejects_paid_core_enablement() -> None:
    with pytest.raises(ValidationError, match="local or zero_price"):
        ComponentPolicyInput(
            component_key="analysis.remote",
            component_version="2026-09",
            cost_class=CostClass.PAID,
            enabled_for_core=True,
            terms_reference="https://example.test/terms",
            reviewed_at=datetime.now(UTC),
        )


def test_component_policy_accepts_explicit_zero_price_component() -> None:
    policy = ComponentPolicyInput(
        component_key="collector.http",
        component_version="1.0.0",
        cost_class=CostClass.ZERO_PRICE,
        enabled_for_core=True,
        terms_reference="https://example.test/terms",
        reviewed_at=datetime.now(UTC),
    )

    assert policy.cost_class is CostClass.ZERO_PRICE
    assert UsageKind.NETWORK_REQUEST == "network_request"
