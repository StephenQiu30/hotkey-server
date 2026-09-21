from __future__ import annotations

import os
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from threading import Barrier
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, text
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
)
from jobs.services import (
    BudgetPolicyConflictError,
    BudgetPolicyUnavailableError,
    BudgetReservationConflictError,
    ComponentPolicyUnavailableError,
    ResourceBudgetService,
    UsageConflictError,
)


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
                "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                "evidence_retention_policies, source_access_policies, "
                "job_stage_attempts, processed_messages, "
                "job_attempts, outbox_messages, jobs, identity_sessions, identity_users"
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
                    "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                    "evidence_retention_policies, source_access_policies, "
                    "job_stage_attempts, processed_messages, job_attempts, outbox_messages, jobs, "
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
) -> BudgetReservationInput:
    return BudgetReservationInput(
        reservation_id=uuid4(),
        operation_id=uuid4(),
        metric=metric,
        requested_units=requested_units,
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
