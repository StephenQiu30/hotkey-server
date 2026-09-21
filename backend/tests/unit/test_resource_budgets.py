from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import pytest
from pydantic import ValidationError

from jobs.execution import resource_attempt_id
from jobs.schemas import (
    BudgetContext,
    BudgetMetric,
    BudgetPolicyInput,
    BudgetScopeKind,
    ComponentPolicyInput,
    CostClass,
    UsageKind,
)


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


def test_global_budget_policy_has_no_scope_reference() -> None:
    policy = BudgetPolicyInput(
        budget_key="global.requests",
        metric=BudgetMetric.NETWORK_REQUEST,
        scope_kind=BudgetScopeKind.GLOBAL,
        scope_reference=None,
        limit_units=10,
        window_seconds=60,
        window_anchor_at=datetime.now(UTC),
        enabled=True,
    )

    assert policy.scope_reference is None

    with pytest.raises(ValidationError, match="global scope"):
        BudgetPolicyInput(
            budget_key="global.requests",
            metric=BudgetMetric.NETWORK_REQUEST,
            scope_kind=BudgetScopeKind.GLOBAL,
            scope_reference="source-a",
            limit_units=10,
            window_seconds=60,
            window_anchor_at=datetime.now(UTC),
            enabled=True,
        )


def test_scoped_budget_policy_requires_stable_reference() -> None:
    with pytest.raises(ValidationError, match="scope_reference"):
        BudgetPolicyInput(
            budget_key="source.requests",
            metric=BudgetMetric.NETWORK_REQUEST,
            scope_kind=BudgetScopeKind.SOURCE,
            scope_reference=None,
            limit_units=10,
            window_seconds=60,
            window_anchor_at=datetime.now(UTC),
            enabled=True,
        )


def test_budget_context_rejects_unstable_references() -> None:
    with pytest.raises(ValidationError, match="stable lowercase"):
        BudgetContext(source_ref="https://example.test/?token=secret")
