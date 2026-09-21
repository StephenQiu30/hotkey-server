from __future__ import annotations

import os
from collections.abc import Iterator
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, text
from sqlalchemy.orm import Session, sessionmaker

from jobs.execution import resource_attempt_id
from jobs.schemas import (
    ComponentPolicyInput,
    CostClass,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
)
from jobs.services import (
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
                "TRUNCATE resource_usage_attempts, resource_component_policies, "
                "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                "evidence_retention_policies, source_access_policies, processed_messages, "
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
                    "TRUNCATE resource_usage_attempts, resource_component_policies, "
                    "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                    "evidence_retention_policies, source_access_policies, "
                    "processed_messages, job_attempts, outbox_messages, jobs, "
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
