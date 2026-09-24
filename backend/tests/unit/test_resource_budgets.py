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
    BudgetReservationInput,
    BudgetScopeKind,
    ComponentPolicyInput,
    CostClass,
    UsageKind,
    XApiPostReadCost,
    XApiUserReadCost,
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
    assert UsageKind.COLLECTOR_CALL == "collector_call"
    assert BudgetMetric.COLLECTOR_CALL == "collector_call"


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


def test_x_api_post_read_cost_reserves_maximum_and_settles_conservatively() -> None:
    cost = XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000)

    assert cost.reservation_units == 50_000
    assert cost.settlement_units(1) == 5000
    assert cost.settlement_units(0) == 0
    assert cost.settlement_units(None) == 50_000

    with pytest.raises(ValueError, match="returned_posts"):
        cost.settlement_units(11)


@pytest.mark.parametrize(
    "values",
    [
        {"max_posts": 0, "unit_price_usd_micros": 5000},
        {"max_posts": 101, "unit_price_usd_micros": 5000},
        {"max_posts": 10, "unit_price_usd_micros": 0},
        {"max_posts": 10, "unit_price_usd_micros": 2**63 - 1},
    ],
)
def test_x_api_post_read_cost_rejects_invalid_or_overflowing_quote(
    values: dict[str, int],
) -> None:
    with pytest.raises(ValidationError):
        XApiPostReadCost(**values)


def test_x_api_user_read_cost_reserves_one_user_and_settles_conservatively() -> None:
    cost = XApiUserReadCost(unit_price_usd_micros=12_345)

    assert cost.reservation_units == 12_345
    assert cost.settlement_units(1) == 12_345
    assert cost.settlement_units(0) == 0
    assert cost.settlement_units(None) == 12_345

    with pytest.raises(ValueError, match="returned_users"):
        cost.settlement_units(2)


def test_x_api_user_read_cost_requires_an_explicit_positive_price() -> None:
    with pytest.raises(ValidationError):
        XApiUserReadCost()

    with pytest.raises(ValidationError):
        XApiUserReadCost(unit_price_usd_micros=0)


def test_x_api_user_read_spend_accepts_a_matching_quote() -> None:
    quote = XApiUserReadCost(unit_price_usd_micros=12_345)

    command = BudgetReservationInput(
        reservation_id=uuid4(),
        operation_id=uuid4(),
        metric=BudgetMetric.X_API_USD_MICROS,
        requested_units=quote.reservation_units,
        context=BudgetContext(source_ref="x"),
        cost_quote=quote,
    )

    assert command.cost_quote == quote


def test_x_api_spend_budget_requires_x_source() -> None:
    with pytest.raises(ValidationError, match="x source"):
        BudgetReservationInput(
            reservation_id=uuid4(),
            operation_id=uuid4(),
            metric=BudgetMetric.X_API_USD_MICROS,
            requested_units=50_000,
            context=BudgetContext(source_ref="bilibili"),
        )

    with pytest.raises(ValidationError, match="x source"):
        BudgetPolicyInput(
            budget_key="source.x.spend",
            metric=BudgetMetric.X_API_USD_MICROS,
            scope_kind=BudgetScopeKind.SOURCE,
            scope_reference="bilibili",
            limit_units=50_000,
            window_seconds=3600,
            window_anchor_at=datetime.now(UTC),
            enabled=True,
        )


@pytest.mark.parametrize(
    "quote",
    [
        None,
        XApiPostReadCost(max_posts=5, unit_price_usd_micros=5000),
        XApiUserReadCost(unit_price_usd_micros=50_001),
    ],
)
def test_x_api_spend_budget_requires_matching_quote(
    quote: XApiPostReadCost | XApiUserReadCost | None,
) -> None:
    with pytest.raises(ValidationError, match="matching cost quote"):
        BudgetReservationInput(
            reservation_id=uuid4(),
            operation_id=uuid4(),
            metric=BudgetMetric.X_API_USD_MICROS,
            requested_units=50_000,
            context=BudgetContext(source_ref="x"),
            cost_quote=quote,
        )


def test_non_x_budget_rejects_cost_quote() -> None:
    with pytest.raises(ValidationError, match="cost quote requires"):
        BudgetReservationInput(
            reservation_id=uuid4(),
            operation_id=uuid4(),
            metric=BudgetMetric.NETWORK_REQUEST,
            requested_units=1,
            context=BudgetContext(source_ref="x"),
            cost_quote=XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000),
        )
