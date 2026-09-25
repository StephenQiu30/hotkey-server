from __future__ import annotations

import os
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from threading import Barrier
from uuid import UUID, uuid4

import httpx
import pytest
from pydantic import SecretStr
from sqlalchemy import Engine, create_engine, text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session, sessionmaker

from jobs.execution import resource_attempt_id
from jobs.schemas import (
    BudgetContext,
    BudgetDecisionStatus,
    BudgetMetric,
    BudgetPolicyInput,
    BudgetReservationInput,
    BudgetResumeCondition,
    BudgetScopeKind,
    ComponentPolicyInput,
    CostClass,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
    XApiPostReadCost,
    XApiUserReadCost,
)
from jobs.services import (
    BudgetPolicyConflictError,
    BudgetPolicyUnavailableError,
    BudgetReservationConflictError,
    ComponentPolicyUnavailableError,
    ResourceBudgetService,
    UsageConflictError,
)
from sources.adapters.x_api import XApiAdapter
from sources.contracts import SearchRequest, SourcePageState, SourceStopReason


@dataclass(frozen=True, slots=True)
class ResourceBudgetContext:
    engine: Engine
    sessions: sessionmaker[Session]
    owner_id: UUID
    other_owner_id: UUID


@dataclass(slots=True)
class MutableClock:
    current: datetime

    def __call__(self) -> datetime:
        return self.current


@pytest.fixture
def resource_budget_context() -> Iterator[ResourceBudgetContext]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    engine = create_engine(database_url)
    sessions = sessionmaker(bind=engine, expire_on_commit=False)
    owner_id = uuid4()
    other_owner_id = uuid4()
    with engine.begin() as connection:
        connection.execute(
            text(
                "TRUNCATE resource_budget_reservations, resource_budget_windows, "
                "resource_budget_policies, resource_usage_attempts, "
                "resource_component_policies, "
                "content_version_relations, content_visibility_observations, "
                "content_observations, content_versions, "
                "content_discoveries, content_threads, content_records, "
                "source_capability_evidence, source_connection_versions, source_connections, "
                "provenance_manifest_inputs, provenance_manifests, "
                "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                "evidence_retention_policies, source_access_policies, "
                "job_stage_attempts, processed_messages, "
                "job_attempts, "
                "outbox_messages, coverage_windows, "
                "jobs, followed_account_aliases, followed_accounts, "
                "monitor_topic_versions, monitor_topics, "
                "identity_sessions, identity_users"
            )
        )
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:owner_id, 'budget-owner', 'test-only-hash', 1, now(), now())"
            ),
            {"owner_id": owner_id},
        )
    try:
        yield ResourceBudgetContext(engine, sessions, owner_id, other_owner_id)
    finally:
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE resource_budget_reservations, resource_budget_windows, "
                    "resource_budget_policies, resource_usage_attempts, "
                    "resource_component_policies, "
                    "content_version_relations, content_visibility_observations, "
                    "content_observations, content_versions, "
                    "content_discoveries, content_threads, content_records, "
                    "source_capability_evidence, source_connection_versions, "
                    "source_connections, provenance_manifest_inputs, provenance_manifests, "
                    "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                    "evidence_retention_policies, source_access_policies, "
                    "job_stage_attempts, processed_messages, job_attempts, "
                    "outbox_messages, coverage_windows, "
                    "jobs, followed_account_aliases, followed_accounts, "
                    "monitor_topic_versions, monitor_topics, "
                    "identity_sessions, identity_users"
                )
            )
        engine.dispose()


def _policy(
    *,
    cost_class: CostClass = CostClass.ZERO_PRICE,
    enabled_for_core: bool = True,
) -> ComponentPolicyInput:
    return ComponentPolicyInput(
        component_key="collector.http",
        component_version="1.0.0",
        cost_class=cost_class,
        enabled_for_core=enabled_for_core,
        terms_reference="https://example.test/terms",
        reviewed_at=datetime.now(UTC),
    )


def _attempt(*, operation_id: UUID, attempt_id: UUID) -> UsageAttemptInput:
    return UsageAttemptInput(
        attempt_id=attempt_id,
        operation_id=operation_id,
        component_key="collector.http",
        usage_kind=UsageKind.NETWORK_REQUEST,
        stage="search.page",
        started_at=datetime.now(UTC),
    )


def _budget_policy(
    *,
    clock: MutableClock,
    budget_key: str = "global.requests",
    metric: BudgetMetric = BudgetMetric.NETWORK_REQUEST,
    scope_kind: BudgetScopeKind = BudgetScopeKind.GLOBAL,
    scope_reference: str | None = None,
    limit_units: int = 1,
    window_seconds: int = 60,
    enabled: bool = True,
) -> BudgetPolicyInput:
    return BudgetPolicyInput(
        budget_key=budget_key,
        metric=metric,
        scope_kind=scope_kind,
        scope_reference=scope_reference,
        limit_units=limit_units,
        window_seconds=window_seconds,
        window_anchor_at=clock.current - timedelta(seconds=10),
        enabled=enabled,
    )


def _reservation(
    *,
    metric: BudgetMetric = BudgetMetric.NETWORK_REQUEST,
    requested_units: int = 1,
    source_ref: str | None = "source-a",
    connection_ref: str | None = "connection-a",
    job_ref: str | None = "job-a",
    cost_quote: XApiPostReadCost | XApiUserReadCost | None = None,
) -> BudgetReservationInput:
    return BudgetReservationInput(
        reservation_id=uuid4(),
        operation_id=uuid4(),
        metric=metric,
        requested_units=requested_units,
        cost_quote=cost_quote,
        context=BudgetContext(
            source_ref=source_ref,
            connection_ref=connection_ref,
            job_ref=job_ref,
        ),
    )


def test_attempt_is_recorded_before_outcome_and_replay_is_idempotent(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    operation_id = uuid4()
    attempt_id = resource_attempt_id(
        operation_id=operation_id,
        component_key="collector.http",
        stage="search.page",
        sequence=1,
    )
    command = _attempt(operation_id=operation_id, attempt_id=attempt_id)

    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session)
        policy_command = _policy()
        policy = service.save_component_policy(
            owner_id=resource_budget_context.owner_id,
            command=policy_command,
        )
        unchanged = service.save_component_policy(
            owner_id=resource_budget_context.owner_id,
            command=policy_command,
        )
        updated = service.save_component_policy(
            owner_id=resource_budget_context.owner_id,
            command=policy_command.model_copy(update={"component_version": "1.0.1"}),
        )
        started = service.begin_attempt(
            owner_id=resource_budget_context.owner_id,
            command=command,
        )
        service.save_component_policy(
            owner_id=resource_budget_context.owner_id,
            command=policy_command.model_copy(
                update={"cost_class": CostClass.PAID, "enabled_for_core": False}
            ),
        )
        replayed = service.begin_attempt(
            owner_id=resource_budget_context.owner_id,
            command=command,
        )

    assert policy.policy_version == 1
    assert unchanged.policy_version == 1
    assert updated.policy_version == 2
    assert started == replayed
    assert started.component_version == "1.0.1"
    assert started.outcome is UsageOutcome.STARTED
    assert started.finished_at is None

    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session)
        finished = service.finish_attempt(
            owner_id=resource_budget_context.owner_id,
            attempt_id=attempt_id,
            outcome=UsageOutcome.EMPTY,
            finished_at=datetime.now(UTC),
        )
        repeated_finish = service.finish_attempt(
            owner_id=resource_budget_context.owner_id,
            attempt_id=attempt_id,
            outcome=UsageOutcome.EMPTY,
            finished_at=finished.finished_at,
        )
        summary = service.usage_summary(
            owner_id=resource_budget_context.owner_id,
            operation_id=operation_id,
        )

    assert repeated_finish == finished
    assert summary.total_attempts == 1
    assert summary.empty_attempts == 1
    assert summary.succeeded_attempts == 0


@pytest.mark.parametrize(
    ("cost_class", "enabled_for_core"),
    [(CostClass.PAID, False), (CostClass.UNKNOWN, False), (CostClass.FREE_CREDIT, False)],
)
def test_non_zero_cost_component_cannot_enter_core_execution(
    resource_budget_context: ResourceBudgetContext,
    cost_class: CostClass,
    enabled_for_core: bool,
) -> None:
    operation_id = uuid4()
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session)
        service.save_component_policy(
            owner_id=resource_budget_context.owner_id,
            command=_policy(cost_class=cost_class, enabled_for_core=enabled_for_core),
        )
        with pytest.raises(ComponentPolicyUnavailableError, match="not enabled"):
            service.begin_attempt(
                owner_id=resource_budget_context.owner_id,
                command=_attempt(operation_id=operation_id, attempt_id=uuid4()),
            )


def test_attempt_replay_with_other_payload_is_rejected(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    operation_id = uuid4()
    attempt_id = uuid4()
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session)
        service.save_component_policy(
            owner_id=resource_budget_context.owner_id,
            command=_policy(),
        )
        service.begin_attempt(
            owner_id=resource_budget_context.owner_id,
            command=_attempt(operation_id=operation_id, attempt_id=attempt_id),
        )
        conflicting = _attempt(operation_id=operation_id, attempt_id=attempt_id).model_copy(
            update={"stage": "detail.page"}
        )
        with pytest.raises(UsageConflictError, match="other data"):
            service.begin_attempt(
                owner_id=resource_budget_context.owner_id,
                command=conflicting,
            )


def test_summary_retains_failed_filtered_empty_and_unfinished_attempts(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    operation_id = uuid4()
    outcomes = (
        UsageOutcome.SUCCEEDED,
        UsageOutcome.FAILED,
        UsageOutcome.FILTERED,
        UsageOutcome.EMPTY,
    )
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session)
        service.save_component_policy(
            owner_id=resource_budget_context.owner_id,
            command=_policy(),
        )
        attempt_ids = [uuid4() for _ in range(len(outcomes) + 1)]
        for attempt_id in attempt_ids:
            service.begin_attempt(
                owner_id=resource_budget_context.owner_id,
                command=_attempt(operation_id=operation_id, attempt_id=attempt_id),
            )
        for attempt_id, outcome in zip(attempt_ids[:-1], outcomes, strict=True):
            service.finish_attempt(
                owner_id=resource_budget_context.owner_id,
                attempt_id=attempt_id,
                outcome=outcome,
                finished_at=datetime.now(UTC),
            )
        summary = service.usage_summary(
            owner_id=resource_budget_context.owner_id,
            operation_id=operation_id,
        )

    assert summary.total_attempts == 5
    assert summary.started_attempts == 1
    assert summary.succeeded_attempts == 1
    assert summary.failed_attempts == 1
    assert summary.filtered_attempts == 1
    assert summary.empty_attempts == 1


def test_component_policy_is_owner_scoped(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session)
        service.save_component_policy(
            owner_id=resource_budget_context.owner_id,
            command=_policy(),
        )
        with pytest.raises(ComponentPolicyUnavailableError, match="not enabled"):
            service.begin_attempt(
                owner_id=resource_budget_context.other_owner_id,
                command=_attempt(operation_id=uuid4(), attempt_id=uuid4()),
            )


def test_last_unit_is_reserved_by_at_most_one_concurrent_transaction(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 21, 12, 0, 10, tzinfo=UTC))
    with resource_budget_context.sessions() as session:
        ResourceBudgetService(session, clock=clock).save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock),
        )

    barrier = Barrier(2)

    def reserve(command: BudgetReservationInput) -> BudgetDecisionStatus:
        with resource_budget_context.sessions() as session:
            barrier.wait()
            return (
                ResourceBudgetService(session, clock=clock)
                .reserve_budget(
                    owner_id=resource_budget_context.owner_id,
                    command=command,
                )
                .status
            )

    commands = (_reservation(), _reservation(job_ref="job-b"))
    with ThreadPoolExecutor(max_workers=2) as executor:
        statuses = list(executor.map(reserve, commands))

    assert statuses.count(BudgetDecisionStatus.RESERVED) == 1
    assert statuses.count(BudgetDecisionStatus.DELAYED) == 1
    with resource_budget_context.engine.connect() as connection:
        reserved_units, reservation_count = connection.execute(
            text(
                "SELECT w.reserved_units, count(r.id) "
                "FROM resource_budget_windows w "
                "JOIN resource_budget_reservations r ON r.budget_window_id = w.id "
                "GROUP BY w.reserved_units"
            )
        ).one()
    assert reserved_units == 1
    assert reservation_count == 1


def test_x_api_spend_last_page_is_reserved_by_one_transaction(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    quote = XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000)
    with resource_budget_context.sessions() as session:
        ResourceBudgetService(session, clock=clock).save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.x.spend",
                metric=BudgetMetric.X_API_USD_MICROS,
                limit_units=quote.reservation_units,
            ),
        )

    barrier = Barrier(2)

    def reserve(command: BudgetReservationInput) -> BudgetDecisionStatus:
        with resource_budget_context.sessions() as session:
            barrier.wait()
            return (
                ResourceBudgetService(session, clock=clock)
                .reserve_budget(owner_id=resource_budget_context.owner_id, command=command)
                .status
            )

    commands = (
        _reservation(
            metric=BudgetMetric.X_API_USD_MICROS,
            requested_units=quote.reservation_units,
            source_ref="x",
            cost_quote=quote,
        ),
        _reservation(
            metric=BudgetMetric.X_API_USD_MICROS,
            requested_units=quote.reservation_units,
            source_ref="x",
            job_ref="job-b",
            cost_quote=quote,
        ),
    )
    with ThreadPoolExecutor(max_workers=2) as executor:
        statuses = list(executor.map(reserve, commands))

    assert statuses.count(BudgetDecisionStatus.RESERVED) == 1
    assert statuses.count(BudgetDecisionStatus.DELAYED) == 1


def test_x_api_request_reserves_network_and_spend_together(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    quote = XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000)
    operation_id, network_id, spend_id = uuid4(), uuid4(), uuid4()
    context = BudgetContext(source_ref="x", job_ref="job-a")
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=1),
        )
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.x.spend",
                metric=BudgetMetric.X_API_USD_MICROS,
                limit_units=quote.reservation_units - 1,
            ),
        )
        with session.begin():
            delayed = service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=spend_id,
                context=context,
                quote=quote,
            )
        assert delayed.status is BudgetDecisionStatus.DELAYED
        assert delayed.metric is BudgetMetric.X_API_USD_MICROS
        assert (
            session.execute(text("SELECT count(*) FROM resource_budget_reservations")).scalar_one()
            == 0
        )
        assert (
            session.execute(text("SELECT count(*) FROM resource_budget_windows")).scalar_one() == 0
        )

        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.x.spend",
                metric=BudgetMetric.X_API_USD_MICROS,
                limit_units=quote.reservation_units,
            ),
        )
        with session.begin():
            reserved = service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=spend_id,
                context=context,
                quote=quote,
            )
        assert reserved.status is BudgetDecisionStatus.RESERVED
        assert reserved.metric is BudgetMetric.X_API_USD_MICROS
        with session.begin():
            replayed = service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=spend_id,
                context=context,
                quote=quote,
            )
        assert replayed.status is BudgetDecisionStatus.RESERVED
        with session.begin():
            service.settle_budget_reservation_in_transaction(
                owner_id=resource_budget_context.owner_id,
                reservation_id=network_id,
                actual_units=1,
            )
            service.settle_budget_reservation_in_transaction(
                owner_id=resource_budget_context.owner_id,
                reservation_id=spend_id,
                actual_units=quote.settlement_units(1),
            )
        with (
            pytest.raises(BudgetReservationConflictError, match="already settled"),
            session.begin(),
        ):
            service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=spend_id,
                context=context,
                quote=quote,
            )
        assert (
            session.execute(text("SELECT count(*) FROM resource_budget_reservations")).scalar_one()
            == 2
        )


def test_x_api_user_read_request_reserves_one_user_and_network_together(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 25, 12, 0, 10, tzinfo=UTC))
    quote = XApiUserReadCost(unit_price_usd_micros=12_345)
    operation_id, network_id, spend_id = uuid4(), uuid4(), uuid4()
    context = BudgetContext(source_ref="x", job_ref="user-lookup")

    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=1),
        )
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.x.spend",
                metric=BudgetMetric.X_API_USD_MICROS,
                limit_units=quote.reservation_units,
            ),
        )
        with session.begin():
            reserved = service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=spend_id,
                context=context,
                quote=quote,
            )

        assert reserved.status is BudgetDecisionStatus.RESERVED
        with session.begin():
            service.settle_budget_reservation_in_transaction(
                owner_id=resource_budget_context.owner_id,
                reservation_id=network_id,
                actual_units=1,
            )
            service.settle_budget_reservation_in_transaction(
                owner_id=resource_budget_context.owner_id,
                reservation_id=spend_id,
                actual_units=quote.settlement_units(1),
            )

        rows = session.execute(
            text(
                "SELECT metric, requested_units, actual_units "
                "FROM resource_budget_reservations ORDER BY metric"
            )
        ).all()

    assert rows == [
        ("network_request", 1, 1),
        ("x_api_usd_micros", quote.reservation_units, quote.unit_price_usd_micros),
    ]


def test_x_api_request_missing_spend_policy_rolls_back_network(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=1),
        )
        with session.begin(), pytest.raises(BudgetPolicyUnavailableError, match="global"):
            service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=uuid4(),
                network_reservation_id=uuid4(),
                spend_reservation_id=uuid4(),
                context=BudgetContext(source_ref="x"),
                quote=XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000),
            )
        assert (
            session.execute(text("SELECT count(*) FROM resource_budget_reservations")).scalar_one()
            == 0
        )
        assert (
            session.execute(text("SELECT count(*) FROM resource_budget_windows")).scalar_one() == 0
        )


@pytest.mark.parametrize(
    "changed",
    [
        XApiPostReadCost(max_posts=5, unit_price_usd_micros=10_000),
        XApiUserReadCost(unit_price_usd_micros=50_000),
    ],
)
def test_x_api_request_replay_rejects_a_different_quote_with_the_same_total(
    resource_budget_context: ResourceBudgetContext,
    changed: XApiPostReadCost | XApiUserReadCost,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    original = XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000)
    operation_id, network_id, spend_id = uuid4(), uuid4(), uuid4()
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=1),
        )
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.x.spend",
                metric=BudgetMetric.X_API_USD_MICROS,
                limit_units=original.reservation_units,
            ),
        )
        with session.begin():
            first = service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=spend_id,
                context=BudgetContext(source_ref="x"),
                quote=original,
            )
        assert first.status is BudgetDecisionStatus.RESERVED
        with session.begin(), pytest.raises(BudgetReservationConflictError):
            service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=spend_id,
                context=BudgetContext(source_ref="x"),
                quote=changed,
            )

        assert session.scalar(text("SELECT count(*) FROM resource_budget_reservations")) == 2


def test_x_api_request_competing_for_last_spend_does_not_consume_extra_network(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    quote = XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000)
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=2),
        )
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.x.spend",
                metric=BudgetMetric.X_API_USD_MICROS,
                limit_units=quote.reservation_units,
            ),
        )

    barrier = Barrier(2)

    def reserve(_: int) -> BudgetDecisionStatus:
        with resource_budget_context.sessions() as session:
            service = ResourceBudgetService(session, clock=clock)
            barrier.wait()
            with session.begin():
                return service.reserve_x_api_request_budgets_in_transaction(
                    owner_id=resource_budget_context.owner_id,
                    operation_id=uuid4(),
                    network_reservation_id=uuid4(),
                    spend_reservation_id=uuid4(),
                    context=BudgetContext(source_ref="x"),
                    quote=quote,
                ).status

    with ThreadPoolExecutor(max_workers=2) as executor:
        statuses = list(executor.map(reserve, (1, 2)))

    assert statuses.count(BudgetDecisionStatus.RESERVED) == 1
    assert statuses.count(BudgetDecisionStatus.DELAYED) == 1
    with resource_budget_context.engine.connect() as connection:
        rows = connection.execute(
            text(
                "SELECT p.metric, w.reserved_units FROM resource_budget_windows w "
                "JOIN resource_budget_policies p ON p.id = w.budget_policy_id"
            )
        ).all()
        reservations = connection.scalar(text("SELECT count(*) FROM resource_budget_reservations"))
    assert dict(rows) == {"network_request": 1, "x_api_usd_micros": 50_000}
    assert reservations == 2


def test_x_api_request_rejects_a_preexisting_single_budget(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    operation_id, network_id = uuid4(), uuid4()
    context = BudgetContext(source_ref="x")
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=2),
        )
        service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=BudgetReservationInput(
                reservation_id=network_id,
                operation_id=operation_id,
                metric=BudgetMetric.NETWORK_REQUEST,
                requested_units=1,
                context=context,
            ),
        )
        with session.begin(), pytest.raises(BudgetReservationConflictError, match="only one"):
            service.reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=uuid4(),
                context=context,
                quote=XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000),
            )
        assert (
            session.execute(text("SELECT count(*) FROM resource_budget_reservations")).scalar_one()
            == 1
        )


def test_x_api_spend_releases_only_verified_unused_resources(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    quote = XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000)
    first = _reservation(
        metric=BudgetMetric.X_API_USD_MICROS,
        requested_units=quote.reservation_units,
        source_ref="x",
        cost_quote=quote,
    )
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        with pytest.raises(BudgetPolicyUnavailableError, match="global"):
            service.reserve_budget(owner_id=resource_budget_context.owner_id, command=first)

        policy = _budget_policy(
            clock=clock,
            budget_key="global.x.spend",
            metric=BudgetMetric.X_API_USD_MICROS,
            limit_units=55_000,
        )
        service.save_budget_policy(owner_id=resource_budget_context.owner_id, command=policy)
        reserved = service.reserve_budget(owner_id=resource_budget_context.owner_id, command=first)
        pending = _reservation(
            metric=BudgetMetric.X_API_USD_MICROS,
            requested_units=quote.reservation_units,
            source_ref="x",
            job_ref="job-b",
            cost_quote=quote,
        )
        delayed = service.reserve_budget(owner_id=resource_budget_context.owner_id, command=pending)
        settled = service.settle_budget_reservation(
            owner_id=resource_budget_context.owner_id,
            reservation_id=first.reservation_id,
            actual_units=quote.settlement_units(1),
        )
        next_page = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=pending,
        )
        unknown = service.settle_budget_reservation(
            owner_id=resource_budget_context.owner_id,
            reservation_id=pending.reservation_id,
            actual_units=quote.settlement_units(None),
        )
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=policy.model_copy(update={"limit_units": 50_000}),
        )
        exhausted = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=_reservation(
                metric=BudgetMetric.X_API_USD_MICROS,
                requested_units=quote.reservation_units,
                source_ref="x",
                job_ref="job-c",
                cost_quote=quote,
            ),
        )

    assert reserved.status is BudgetDecisionStatus.RESERVED
    assert delayed.status is BudgetDecisionStatus.DELAYED
    assert settled.actual_units == 5000
    assert settled.released_units == 45_000
    assert next_page.status is BudgetDecisionStatus.RESERVED
    assert unknown.actual_units == 50_000
    assert unknown.released_units == 0
    assert exhausted.status is BudgetDecisionStatus.DELAYED


def test_x_api_spend_rejects_unvalidated_model_copies(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    policy = _budget_policy(
        clock=clock,
        budget_key="source.x.spend",
        metric=BudgetMetric.X_API_USD_MICROS,
        scope_kind=BudgetScopeKind.SOURCE,
        scope_reference="x",
        limit_units=50_000,
    )
    reservation = _reservation(
        metric=BudgetMetric.X_API_USD_MICROS,
        requested_units=50_000,
        source_ref="x",
        cost_quote=XApiPostReadCost(max_posts=10, unit_price_usd_micros=5000),
    )
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        with pytest.raises(ValueError, match="x source"):
            service.save_budget_policy(
                owner_id=resource_budget_context.owner_id,
                command=policy.model_copy(update={"scope_reference": "bilibili"}),
            )
        with pytest.raises(ValueError, match="x source"):
            service.reserve_budget(
                owner_id=resource_budget_context.owner_id,
                command=reservation.model_copy(
                    update={
                        "metric": "x_api_usd_micros",
                        "context": BudgetContext(source_ref="bilibili"),
                    }
                ),
            )

    with (
        pytest.raises(IntegrityError, match="resource_budget_policies_x_source_check"),
        resource_budget_context.engine.begin() as connection,
    ):
        connection.execute(
            text(
                "INSERT INTO resource_budget_policies "
                "(id, owner_id, budget_key, metric, scope_kind, scope_reference, "
                "limit_units, window_seconds, window_anchor_at, enabled, "
                "created_at, updated_at) "
                "VALUES (:id, :owner_id, 'source.invalid.x.spend', "
                "'x_api_usd_micros', 'source', 'bilibili', "
                "50000, 3600, now(), true, now(), now())"
            ),
            {"id": uuid4(), "owner_id": resource_budget_context.owner_id},
        )


def test_offline_x_adapter_uses_persistent_spend_window_per_page(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    operation_id = uuid4()
    reservations: dict[int, tuple[UUID, UUID, XApiPostReadCost]] = {}
    requests: list[httpx.Request] = []
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=2),
        )
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.x.spend",
                metric=BudgetMetric.X_API_USD_MICROS,
                limit_units=55_000,
            ),
        )

    def authorize(attempt: int, max_posts: int) -> bool:
        cost = XApiPostReadCost(max_posts=max_posts, unit_price_usd_micros=5000)
        network_id, spend_id = uuid4(), uuid4()
        with resource_budget_context.sessions() as session, session.begin():
            decision = ResourceBudgetService(
                session, clock=clock
            ).reserve_x_api_request_budgets_in_transaction(
                owner_id=resource_budget_context.owner_id,
                operation_id=operation_id,
                network_reservation_id=network_id,
                spend_reservation_id=spend_id,
                context=BudgetContext(source_ref="x"),
                quote=cost,
            )
        if decision.status is BudgetDecisionStatus.RESERVED:
            reservations[attempt] = (network_id, spend_id, cost)
            return True
        return False

    def settle(attempt: int, posts: int | None) -> None:
        network_id, spend_id, cost = reservations[attempt]
        with resource_budget_context.sessions() as session:
            service = ResourceBudgetService(session, clock=clock)
            with session.begin():
                service.settle_budget_reservation_in_transaction(
                    owner_id=resource_budget_context.owner_id,
                    reservation_id=network_id,
                    actual_units=1,
                )
                service.settle_budget_reservation_in_transaction(
                    owner_id=resource_budget_context.owner_id,
                    reservation_id=spend_id,
                    actual_units=cost.settlement_units(posts),
                )

    def respond(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if len(requests) == 1:
            return httpx.Response(
                200,
                json={
                    "data": [{"id": "1", "author_id": "42", "text": "post"}],
                    "meta": {"result_count": 1, "next_token": "ABCD1234"},
                },
            )
        return httpx.Response(429)

    adapter = XApiAdapter(
        token=SecretStr("offline-test-token"),
        transport=httpx.MockTransport(respond),
        authorize_request=authorize,
        settle_request=settle,
    )
    first = adapter.fetch_page(SearchRequest(source_key="x", query="topic", page_size=10))
    second = adapter.fetch_page(
        SearchRequest(source_key="x", query="topic", page_size=10, page_token="ABCD1234")
    )

    assert first.state is SourcePageState.MORE
    assert second.stop_reason is SourceStopReason.RATE_LIMITED
    assert len(requests) == 2
    assert all("expansions" not in request.url.params for request in requests)
    with resource_budget_context.engine.connect() as connection:
        used, reserved = connection.execute(
            text(
                "SELECT used_units, reserved_units FROM resource_budget_windows "
                "WHERE budget_mode = 'cumulative' AND "
                "budget_policy_id IN (SELECT id FROM resource_budget_policies "
                "WHERE metric = 'x_api_usd_micros')"
            )
        ).one()
        network_used = connection.scalar(
            text(
                "SELECT used_units FROM resource_budget_windows WHERE budget_policy_id "
                "IN (SELECT id FROM resource_budget_policies WHERE metric = 'network_request')"
            )
        )
    assert (used, reserved) == (55_000, 0)
    assert network_used == 2


def test_strictest_scope_delays_without_partial_reservation(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 21, 12, 0, 10, tzinfo=UTC))
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=2),
        )
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="source.requests",
                scope_kind=BudgetScopeKind.SOURCE,
                scope_reference="source-a",
            ),
        )
        first = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=_reservation(),
        )
        delayed = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=_reservation(job_ref="job-b"),
        )

    assert first.status is BudgetDecisionStatus.RESERVED
    assert delayed.status is BudgetDecisionStatus.DELAYED
    assert delayed.remaining_units == 0
    assert delayed.limiting_budget_keys == ("source.requests",)
    assert delayed.resume_condition is BudgetResumeCondition.NEXT_WINDOW
    assert delayed.retry_at == datetime(2026, 9, 21, 12, 1, tzinfo=UTC)
    with resource_budget_context.engine.connect() as connection:
        counters = connection.execute(
            text(
                "SELECT p.budget_key, w.reserved_units "
                "FROM resource_budget_windows w "
                "JOIN resource_budget_policies p ON p.id = w.budget_policy_id "
                "ORDER BY p.budget_key"
            )
        ).all()
    assert counters == [("global.requests", 1), ("source.requests", 1)]


def test_cumulative_settlement_releases_unused_units_and_is_idempotent(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 21, 12, 0, 10, tzinfo=UTC))
    command = _reservation(requested_units=2)
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock, limit_units=2),
        )
        reserved = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=command,
        )
        replayed = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=command,
        )
        settled = service.settle_budget_reservation(
            owner_id=resource_budget_context.owner_id,
            reservation_id=command.reservation_id,
            actual_units=1,
        )
        repeated = service.settle_budget_reservation(
            owner_id=resource_budget_context.owner_id,
            reservation_id=command.reservation_id,
            actual_units=1,
        )
        next_unit = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=_reservation(),
        )

    assert replayed == reserved
    assert settled == repeated
    assert settled.actual_units == 1
    assert settled.released_units == 1
    assert next_unit.status is BudgetDecisionStatus.RESERVED


def test_concurrency_slot_is_reusable_only_after_settlement(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 21, 12, 0, 10, tzinfo=UTC))
    first_command = _reservation(metric=BudgetMetric.CONCURRENCY_SLOT)
    second_command = _reservation(metric=BudgetMetric.CONCURRENCY_SLOT, job_ref="job-b")
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.concurrent",
                metric=BudgetMetric.CONCURRENCY_SLOT,
            ),
        )
        first = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=first_command,
        )
        delayed = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=second_command,
        )
        service.settle_budget_reservation(
            owner_id=resource_budget_context.owner_id,
            reservation_id=first_command.reservation_id,
            actual_units=1,
        )
        retried = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=second_command,
        )

    assert first.status is BudgetDecisionStatus.RESERVED
    assert delayed.status is BudgetDecisionStatus.DELAYED
    assert delayed.resume_condition is BudgetResumeCondition.CAPACITY_RELEASE
    assert delayed.retry_at is None
    assert retried.status is BudgetDecisionStatus.RESERVED
    with resource_budget_context.engine.connect() as connection:
        used_units, reserved_units = connection.execute(
            text(
                "SELECT used_units, reserved_units FROM resource_budget_windows "
                "ORDER BY created_at LIMIT 1"
            )
        ).one()
    assert used_units == 0
    assert reserved_units == 1


def test_restart_preserves_exhaustion_until_the_next_window(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 21, 12, 0, 10, tzinfo=UTC))
    first_command = _reservation()
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock),
        )
        service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=first_command,
        )
        service.settle_budget_reservation(
            owner_id=resource_budget_context.owner_id,
            reservation_id=first_command.reservation_id,
            actual_units=1,
        )

    with resource_budget_context.sessions() as restarted_session:
        restarted = ResourceBudgetService(restarted_session, clock=clock)
        delayed = restarted.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=_reservation(job_ref="job-after-restart"),
        )
        clock.current = datetime(2026, 9, 21, 12, 1, 1, tzinfo=UTC)
        next_window = restarted.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=_reservation(job_ref="job-next-window"),
        )

    assert delayed.status is BudgetDecisionStatus.DELAYED
    assert delayed.retry_at == datetime(2026, 9, 21, 12, 1, tzinfo=UTC)
    assert next_window.status is BudgetDecisionStatus.RESERVED


def test_lower_limit_does_not_reset_current_window_and_structure_is_immutable(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 21, 12, 0, 10, tzinfo=UTC))
    initial_policy = _budget_policy(clock=clock, limit_units=2)
    first_command = _reservation()
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=initial_policy,
        )
        service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=first_command,
        )
        service.settle_budget_reservation(
            owner_id=resource_budget_context.owner_id,
            reservation_id=first_command.reservation_id,
            actual_units=1,
        )
        updated = service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=initial_policy.model_copy(update={"limit_units": 1}),
        )
        delayed = service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=_reservation(job_ref="job-after-limit-change"),
        )
        with pytest.raises(BudgetPolicyConflictError, match="structural"):
            service.save_budget_policy(
                owner_id=resource_budget_context.owner_id,
                command=initial_policy.model_copy(update={"window_seconds": 120}),
            )

    assert updated.policy_version == 2
    assert delayed.status is BudgetDecisionStatus.DELAYED


def test_missing_global_budget_fails_closed(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 21, 12, 0, 10, tzinfo=UTC))
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="source.requests",
                scope_kind=BudgetScopeKind.SOURCE,
                scope_reference="source-a",
            ),
        )
        with pytest.raises(BudgetPolicyUnavailableError, match="global"):
            service.reserve_budget(
                owner_id=resource_budget_context.owner_id,
                command=_reservation(),
            )


def test_reservation_and_settlement_replays_reject_conflicting_data(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 21, 12, 0, 10, tzinfo=UTC))
    command = _reservation()
    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=resource_budget_context.owner_id,
            command=_budget_policy(clock=clock),
        )
        service.reserve_budget(
            owner_id=resource_budget_context.owner_id,
            command=command,
        )
        with pytest.raises(BudgetReservationConflictError, match="other data"):
            service.reserve_budget(
                owner_id=resource_budget_context.owner_id,
                command=command.model_copy(
                    update={"context": BudgetContext(source_ref="source-b")}
                ),
            )
        service.settle_budget_reservation(
            owner_id=resource_budget_context.owner_id,
            reservation_id=command.reservation_id,
            actual_units=1,
        )
        with pytest.raises(BudgetReservationConflictError, match="settlement"):
            service.settle_budget_reservation(
                owner_id=resource_budget_context.owner_id,
                reservation_id=command.reservation_id,
                actual_units=0,
            )


def test_budget_usage_snapshot_is_owner_scoped_and_read_only(
    resource_budget_context: ResourceBudgetContext,
) -> None:
    clock = MutableClock(datetime(2026, 9, 23, 12, 0, 10, tzinfo=UTC))
    owner_id = resource_budget_context.owner_id
    other_owner_id = resource_budget_context.other_owner_id

    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        service.save_budget_policy(
            owner_id=owner_id,
            command=_budget_policy(clock=clock, budget_key="global.requests", limit_units=10),
        )
        service.save_budget_policy(
            owner_id=owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.concurrent",
                metric=BudgetMetric.CONCURRENCY_SLOT,
                limit_units=4,
            ),
        )
        service.save_budget_policy(
            owner_id=owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.analysis",
                metric=BudgetMetric.ANALYSIS_ATTEMPT,
                limit_units=20,
            ),
        )
        service.save_budget_policy(
            owner_id=owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="source.concurrent",
                metric=BudgetMetric.CONCURRENCY_SLOT,
                scope_kind=BudgetScopeKind.SOURCE,
                scope_reference="source-a",
                limit_units=3,
            ),
        )
        lowering_policy = _budget_policy(
            clock=clock,
            budget_key="source.lowered",
            metric=BudgetMetric.ANALYSIS_ATTEMPT,
            scope_kind=BudgetScopeKind.SOURCE,
            scope_reference="source-c",
            limit_units=9,
        )
        service.save_budget_policy(owner_id=owner_id, command=lowering_policy)
        future_policy = _budget_policy(
            clock=clock,
            budget_key="source.future",
            metric=BudgetMetric.ANALYSIS_ATTEMPT,
            scope_kind=BudgetScopeKind.SOURCE,
            scope_reference="source-b",
            limit_units=8,
        ).model_copy(update={"window_anchor_at": clock.current + timedelta(minutes=5)})
        service.save_budget_policy(owner_id=owner_id, command=future_policy)
        service.save_budget_policy(
            owner_id=owner_id,
            command=_budget_policy(
                clock=clock,
                budget_key="global.disabled",
                metric=BudgetMetric.COLLECTOR_CALL,
                enabled=False,
            ),
        )
        request = _reservation(requested_units=4)
        service.reserve_budget(owner_id=owner_id, command=request)
        service.settle_budget_reservation(
            owner_id=owner_id,
            reservation_id=request.reservation_id,
            actual_units=2,
        )
        consumed = _reservation(
            metric=BudgetMetric.ANALYSIS_ATTEMPT,
            requested_units=5,
            source_ref="source-c",
            connection_ref=None,
            job_ref=None,
        )
        service.reserve_budget(owner_id=owner_id, command=consumed)
        service.settle_budget_reservation(
            owner_id=owner_id,
            reservation_id=consumed.reservation_id,
            actual_units=4,
        )
        service.save_budget_policy(
            owner_id=owner_id,
            command=lowering_policy.model_copy(update={"limit_units": 3}),
        )
        service.reserve_budget(
            owner_id=owner_id,
            command=_reservation(
                metric=BudgetMetric.CONCURRENCY_SLOT,
                requested_units=2,
                source_ref="source-a",
                connection_ref=None,
                job_ref=None,
            ),
        )

    with resource_budget_context.engine.connect() as connection:
        before = connection.execute(
            text(
                "SELECT "
                "(SELECT count(*) FROM resource_budget_windows), "
                "(SELECT count(*) FROM resource_budget_reservations)"
            )
        ).one()

    with resource_budget_context.sessions() as session:
        service = ResourceBudgetService(session, clock=clock)
        snapshot = service.budget_usage_snapshot(owner_id=owner_id)
        other_snapshot = service.budget_usage_snapshot(owner_id=other_owner_id)

    with resource_budget_context.engine.connect() as connection:
        after = connection.execute(
            text(
                "SELECT "
                "(SELECT count(*) FROM resource_budget_windows), "
                "(SELECT count(*) FROM resource_budget_reservations)"
            )
        ).one()

    by_key = {row.budget_key: row for row in snapshot}
    assert tuple(row.budget_key for row in snapshot) == (
        "global.analysis",
        "source.future",
        "source.lowered",
        "global.disabled",
        "global.concurrent",
        "source.concurrent",
        "global.requests",
    )
    assert by_key["global.requests"].used_units == 2
    assert by_key["global.requests"].reserved_units == 0
    assert by_key["global.requests"].remaining_units == 8
    assert by_key["source.concurrent"].used_units == 0
    assert by_key["source.concurrent"].reserved_units == 2
    assert by_key["source.concurrent"].remaining_units == 1
    assert by_key["global.concurrent"].remaining_units == 2
    assert by_key["source.future"].next_window_at == clock.current + timedelta(minutes=5)
    assert by_key["source.future"].window_start is None
    assert by_key["source.future"].remaining_units is None
    assert by_key["source.lowered"].used_units == 4
    assert by_key["source.lowered"].remaining_units == 0
    assert by_key["source.lowered"].policy_version == 2
    assert by_key["global.disabled"].enabled is False
    assert by_key["global.disabled"].remaining_units is None
    assert other_snapshot == ()
    assert before == after
