from __future__ import annotations

import os
from collections.abc import Iterator
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from pydantic import ValidationError
from sqlalchemy import Engine, create_engine, inspect, text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session, sessionmaker

from db.metadata import metadata
from jobs.schemas import (
    ComponentPolicyInput,
    CostClass,
    JobAcceptanceInput,
    JobObservationContext,
    JobStage,
    JobStageOutcome,
    OperationalTaskStatus,
    StageAttemptInput,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
)
from jobs.services import (
    JobObservationService,
    JobService,
    ResourceBudgetService,
    StageAttemptConflictError,
)
from sources.contracts import SourceCapability

WINDOW_START = datetime(2026, 9, 21, 12, tzinfo=UTC)


@dataclass(frozen=True, slots=True)
class ObservationTestContext:
    engine: Engine
    sessions: sessionmaker[Session]
    owner_id: UUID
    other_owner_id: UUID


@pytest.fixture
def observation_context() -> Iterator[ObservationTestContext]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    engine = create_engine(database_url, pool_size=4)
    sessions = sessionmaker(bind=engine, expire_on_commit=False)
    owner_id = uuid4()
    other_owner_id = uuid4()
    with engine.begin() as connection:
        connection.execute(
            text(
                "TRUNCATE content_version_relations, content_visibility_observations, "
                "content_observations, content_versions, "
                "content_discoveries, content_records, "
                "source_capability_evidence, source_connection_versions, "
                "source_connections, provenance_manifest_inputs, provenance_manifests, "
                "evidence_cleanup_targets, evidence_deletions, "
                "evidence_resources, evidence_retention_policies, source_access_policies, "
                "resource_budget_reservations, resource_budget_windows, "
                "resource_budget_policies, resource_usage_attempts, "
                "resource_component_policies, job_stage_attempts, "
                "processed_messages, job_attempts, "
                "outbox_messages, coverage_windows, "
                "jobs, monitor_topic_versions, monitor_topics, "
                "identity_sessions, identity_users"
            )
        )
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:owner_id, 'observation-owner', 'test-only-hash', 1, now(), now())"
            ),
            {"owner_id": owner_id},
        )
    try:
        yield ObservationTestContext(
            engine=engine,
            sessions=sessions,
            owner_id=owner_id,
            other_owner_id=other_owner_id,
        )
    finally:
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE content_version_relations, content_visibility_observations, "
                    "content_observations, content_versions, "
                    "content_discoveries, content_records, "
                    "source_capability_evidence, source_connection_versions, "
                    "source_connections, provenance_manifest_inputs, provenance_manifests, "
                    "evidence_cleanup_targets, evidence_deletions, "
                    "evidence_resources, evidence_retention_policies, source_access_policies, "
                    "resource_budget_reservations, resource_budget_windows, "
                    "resource_budget_policies, resource_usage_attempts, "
                    "resource_component_policies, job_stage_attempts, "
                    "processed_messages, job_attempts, "
                    "outbox_messages, coverage_windows, "
                    "jobs, monitor_topic_versions, monitor_topics, "
                    "identity_sessions, identity_users"
                )
            )
        engine.dispose()


def _observation(version: int = 1) -> JobObservationContext:
    return JobObservationContext(
        configuration_ref="monitor-config-1",
        configuration_version=version,
        source_key="x",
        source_capability=SourceCapability.SEARCH,
    )


def _command(*, version: int = 1) -> JobAcceptanceInput:
    return JobAcceptanceInput(
        operation_id=uuid4(),
        kind="monitor.collect",
        observation=_observation(version),
        scope={"window": version},
    )


def test_observation_inputs_reject_sensitive_or_ambiguous_context() -> None:
    with pytest.raises(ValidationError):
        JobObservationContext(
            configuration_ref="monitor-config-1",
            configuration_version=1,
            source_key="x",
            source_capability=None,
        )
    with pytest.raises(ValidationError):
        JobObservationContext(
            configuration_ref="monitor-config-1",
            configuration_version=1,
            source_key="x",
            source_capability=SourceCapability.SEARCH,
            keyword="sensitive query",
        )
    with pytest.raises(ValidationError):
        StageAttemptInput(
            attempt_id=uuid4(),
            stage=JobStage.REQUEST,
            attempt_sequence=1,
            started_at=WINDOW_START,
            details={"response_body": "must not be persisted"},
        )


def test_observation_ddl_matches_runtime_models(
    observation_context: ObservationTestContext,
) -> None:
    inspector = inspect(observation_context.engine)

    for table_name in ("jobs", "job_stage_attempts"):
        database_columns = {
            column["name"] for column in inspector.get_columns(table_name, schema="public")
        }
        model_columns = set(metadata.tables[table_name].columns.keys())
        assert database_columns == model_columns


def test_context_and_stage_attempts_are_persistent_idempotent_facts(
    observation_context: ObservationTestContext,
) -> None:
    command = _command(version=3)
    with observation_context.sessions() as session:
        job = JobService(session, clock=lambda: WINDOW_START).accept(
            owner_id=observation_context.owner_id,
            command=command,
        )

    assert job.observation == command.observation
    with observation_context.engine.connect() as connection:
        payload = connection.execute(
            text("SELECT payload FROM outbox_messages WHERE aggregate_id = :job_id"),
            {"job_id": job.id},
        ).scalar_one()
    assert payload["configuration_ref"] == "monitor-config-1"
    assert payload["configuration_version"] == 3
    assert payload["source_key"] == "x"
    assert payload["source_capability"] == "search"

    with (
        pytest.raises(IntegrityError),
        observation_context.engine.begin() as connection,
    ):
        connection.execute(
            text("UPDATE jobs SET source_capability = NULL WHERE id = :job_id"),
            {"job_id": job.id},
        )
    with (
        pytest.raises(IntegrityError),
        observation_context.engine.begin() as connection,
    ):
        connection.execute(
            text("UPDATE jobs SET next_run_at = :next_run_at WHERE id = :job_id"),
            {"job_id": job.id, "next_run_at": WINDOW_START + timedelta(minutes=1)},
        )

    with observation_context.sessions() as session, pytest.raises(ValueError):
        JobObservationService(session).start_stage(
            owner_id=observation_context.owner_id,
            job_id=job.id,
            command=StageAttemptInput(
                attempt_id=uuid4(),
                stage=JobStage.REQUEST,
                attempt_sequence=1,
                started_at=WINDOW_START - timedelta(seconds=1),
            ),
        )

    attempt = StageAttemptInput(
        attempt_id=uuid4(),
        stage=JobStage.REQUEST,
        attempt_sequence=1,
        started_at=WINDOW_START + timedelta(seconds=1),
    )
    with observation_context.sessions() as session:
        service = JobObservationService(session)
        started = service.start_stage(
            owner_id=observation_context.owner_id,
            job_id=job.id,
            command=attempt,
        )
    with observation_context.sessions() as session:
        replayed = JobObservationService(session).start_stage(
            owner_id=observation_context.owner_id,
            job_id=job.id,
            command=attempt,
        )
    assert replayed == started

    conflicting = attempt.model_copy(update={"stage": JobStage.PARSE})
    with (
        observation_context.sessions() as session,
        pytest.raises(StageAttemptConflictError),
    ):
        JobObservationService(session).start_stage(
            owner_id=observation_context.owner_id,
            job_id=job.id,
            command=conflicting,
        )

    finished_at = WINDOW_START + timedelta(seconds=2)
    with observation_context.sessions() as session:
        finished = JobObservationService(session).finish_stage(
            owner_id=observation_context.owner_id,
            attempt_id=attempt.attempt_id,
            outcome=JobStageOutcome.SUCCEEDED,
            finished_at=finished_at,
        )
    with observation_context.sessions() as session:
        replayed_finish = JobObservationService(session).finish_stage(
            owner_id=observation_context.owner_id,
            attempt_id=attempt.attempt_id,
            outcome=JobStageOutcome.SUCCEEDED,
            finished_at=finished_at,
        )
    assert replayed_finish == finished

    with (
        observation_context.sessions() as session,
        pytest.raises(StageAttemptConflictError),
    ):
        JobObservationService(session).finish_stage(
            owner_id=observation_context.owner_id,
            attempt_id=attempt.attempt_id,
            outcome=JobStageOutcome.SUCCEEDED,
            finished_at=finished_at + timedelta(seconds=1),
        )

    with (
        observation_context.sessions() as session,
        pytest.raises(StageAttemptConflictError),
    ):
        JobObservationService(session).finish_stage(
            owner_id=observation_context.owner_id,
            attempt_id=attempt.attempt_id,
            outcome=JobStageOutcome.FAILED,
            finished_at=finished_at,
        )


def test_snapshot_reconciles_mutually_exclusive_tasks_and_separate_attempts(
    observation_context: ObservationTestContext,
) -> None:
    jobs = []
    with observation_context.sessions() as session:
        service = JobService(session, clock=lambda: WINDOW_START)
        for version in range(1, 8):
            jobs.append(
                service.accept(
                    owner_id=observation_context.owner_id,
                    command=_command(version=version),
                )
            )

    states = (
        ("queued", None, None),
        ("queued", "budget_exhausted", WINDOW_START + timedelta(hours=1)),
        ("running", None, None),
        ("succeeded", None, None),
        ("partially_succeeded", None, None),
        ("failed", None, None),
        ("cancelled", None, None),
    )
    with observation_context.engine.begin() as connection:
        for index, (job, state) in enumerate(zip(jobs, states, strict=True)):
            status, defer_reason, next_run_at = state
            terminal = status in {
                "succeeded",
                "partially_succeeded",
                "failed",
                "cancelled",
            }
            connection.execute(
                text(
                    "UPDATE jobs SET status = :status, defer_reason = :defer_reason, "
                    "next_run_at = :next_run_at, started_at = :started_at, "
                    "completed_at = :completed_at, updated_at = :updated_at WHERE id = :job_id"
                ),
                {
                    "status": status,
                    "defer_reason": defer_reason,
                    "next_run_at": next_run_at,
                    "started_at": None if index < 2 else WINDOW_START + timedelta(seconds=1),
                    "completed_at": (WINDOW_START + timedelta(seconds=5) if terminal else None),
                    "updated_at": WINDOW_START + timedelta(seconds=5),
                    "job_id": job.id,
                },
            )
        connection.execute(
            text(
                "INSERT INTO job_attempts "
                "(id, job_id, lease_epoch, worker_id, started_at, lease_expires_at, "
                "finished_at, outcome) VALUES "
                "(:first_id, :job_id, 1, 'worker-1', :started_at, :expires_at, "
                ":finished_at, 'expired'), "
                "(:second_id, :job_id, 2, 'worker-2', :started_at, :expires_at, "
                ":finished_at, 'succeeded')"
            ),
            {
                "first_id": uuid4(),
                "second_id": uuid4(),
                "job_id": jobs[3].id,
                "started_at": WINDOW_START + timedelta(seconds=1),
                "expires_at": WINDOW_START + timedelta(minutes=1),
                "finished_at": WINDOW_START + timedelta(seconds=5),
            },
        )

    for sequence, outcome in (
        (1, JobStageOutcome.FAILED),
        (2, JobStageOutcome.SUCCEEDED),
    ):
        attempt_id = uuid4()
        with observation_context.sessions() as session:
            observation = JobObservationService(session)
            observation.start_stage(
                owner_id=observation_context.owner_id,
                job_id=jobs[3].id,
                command=StageAttemptInput(
                    attempt_id=attempt_id,
                    stage=JobStage.REQUEST,
                    attempt_sequence=sequence,
                    started_at=WINDOW_START + timedelta(seconds=sequence),
                ),
            )
        with observation_context.sessions() as session:
            JobObservationService(session).finish_stage(
                owner_id=observation_context.owner_id,
                attempt_id=attempt_id,
                outcome=outcome,
                finished_at=WINDOW_START + timedelta(seconds=sequence + 1),
            )

    with observation_context.sessions() as session:
        resources = ResourceBudgetService(session, clock=lambda: WINDOW_START)
        resources.save_component_policy(
            owner_id=observation_context.owner_id,
            command=ComponentPolicyInput(
                component_key="source.x",
                component_version="1.0.0",
                cost_class=CostClass.LOCAL,
                enabled_for_core=True,
                terms_reference="https://example.invalid/terms",
                reviewed_at=WINDOW_START,
            ),
        )
    resource_attempt_id = uuid4()
    with observation_context.sessions() as session:
        resources = ResourceBudgetService(session)
        resources.begin_attempt(
            owner_id=observation_context.owner_id,
            command=UsageAttemptInput(
                attempt_id=resource_attempt_id,
                operation_id=jobs[3].operation_id,
                component_key="source.x",
                usage_kind=UsageKind.NETWORK_REQUEST,
                stage="search.page",
                started_at=WINDOW_START + timedelta(seconds=1),
            ),
        )
        resources.finish_attempt(
            owner_id=observation_context.owner_id,
            attempt_id=resource_attempt_id,
            outcome=UsageOutcome.SUCCEEDED,
            finished_at=WINDOW_START + timedelta(seconds=2),
        )

    with observation_context.sessions() as session:
        snapshot = JobObservationService(session).snapshot(
            owner_id=observation_context.owner_id,
            window_start=WINDOW_START - timedelta(seconds=1),
            window_end=WINDOW_START + timedelta(minutes=1),
        )

    assert snapshot.summary.total_tasks == 7
    assert snapshot.summary.count(OperationalTaskStatus.QUEUED) == 1
    assert snapshot.summary.count(OperationalTaskStatus.DELAYED) == 1
    assert snapshot.summary.count(OperationalTaskStatus.RUNNING) == 1
    assert snapshot.summary.count(OperationalTaskStatus.SUCCEEDED) == 1
    assert snapshot.summary.count(OperationalTaskStatus.PARTIALLY_SUCCEEDED) == 1
    assert snapshot.summary.count(OperationalTaskStatus.FAILED) == 1
    assert snapshot.summary.count(OperationalTaskStatus.CANCELLED) == 1
    assert sum(snapshot.summary.task_counts.values()) == snapshot.summary.total_tasks
    assert snapshot.summary.execution_attempts == 2
    assert snapshot.summary.stage_attempts == 2
    assert snapshot.summary.resource_attempts == 1
    assert sum(record.execution_attempts for record in snapshot.tasks) == 2
    assert sum(record.stage_attempts for record in snapshot.tasks) == 2
    assert sum(item.attempts for item in snapshot.operations) == 1
    assert {record.observation.configuration_version for record in snapshot.tasks} == set(
        range(1, 8)
    )
    assert not hasattr(snapshot.tasks[0], "scope")


def test_snapshot_is_owner_scoped_and_uses_a_half_open_window(
    observation_context: ObservationTestContext,
) -> None:
    with observation_context.sessions() as session:
        own = JobService(session, clock=lambda: WINDOW_START).accept(
            owner_id=observation_context.owner_id,
            command=_command(),
        )
    with observation_context.sessions() as session:
        visible = JobObservationService(session).snapshot(
            owner_id=observation_context.owner_id,
            window_start=WINDOW_START,
            window_end=WINDOW_START + timedelta(seconds=1),
        )
        empty = JobObservationService(session).snapshot(
            owner_id=observation_context.owner_id,
            window_start=WINDOW_START - timedelta(seconds=1),
            window_end=WINDOW_START,
        )
        other_owner = JobObservationService(session).snapshot(
            owner_id=observation_context.other_owner_id,
            window_start=WINDOW_START,
            window_end=WINDOW_START + timedelta(seconds=1),
        )

    assert [record.job_id for record in visible.tasks] == [own.id]
    assert visible.summary.total_tasks == 1
    assert empty.summary.total_tasks == 0
    assert empty.summary.execution_attempts == 0
    assert empty.summary.stage_attempts == 0
    assert empty.summary.resource_attempts == 0
    assert other_owner.summary.total_tasks == 0


def test_snapshot_keeps_source_capability_states_separate(
    observation_context: ObservationTestContext,
) -> None:
    cases = (
        ("x", SourceCapability.SEARCH, "failed"),
        ("x", SourceCapability.SEARCH, "succeeded"),
        ("x", SourceCapability.COMMENTS, "succeeded"),
        ("bilibili", SourceCapability.SEARCH, "partially_succeeded"),
        (None, None, "queued"),
    )
    with observation_context.sessions() as session:
        service = JobService(session, clock=lambda: WINDOW_START)
        jobs = [
            service.accept(
                owner_id=observation_context.owner_id,
                command=JobAcceptanceInput(
                    operation_id=uuid4(),
                    kind="monitor.collect",
                    observation=JobObservationContext(
                        configuration_ref="monitor-config-1",
                        configuration_version=1,
                        source_key=source_key,
                        source_capability=capability,
                    ),
                    scope={},
                ),
            )
            for source_key, capability, _ in cases
        ]
    with observation_context.engine.begin() as connection:
        for job, (_, _, status) in zip(jobs, cases, strict=True):
            if status == "queued":
                continue
            connection.execute(
                text(
                    "UPDATE jobs SET status = :status, started_at = :started_at, "
                    "completed_at = :completed_at, updated_at = :completed_at WHERE id = :job_id"
                ),
                {
                    "status": status,
                    "started_at": WINDOW_START + timedelta(seconds=1),
                    "completed_at": WINDOW_START + timedelta(seconds=2),
                    "job_id": job.id,
                },
            )

    with observation_context.sessions() as session:
        snapshot = JobObservationService(session).snapshot(
            owner_id=observation_context.owner_id,
            window_start=WINDOW_START,
            window_end=WINDOW_START + timedelta(minutes=1),
        )
        other_owner = JobObservationService(session).snapshot(
            owner_id=observation_context.other_owner_id,
            window_start=WINDOW_START,
            window_end=WINDOW_START + timedelta(minutes=1),
        )

    assert snapshot.summary.total_tasks == 5
    assert [
        (item.source_key, item.source_capability, item.total_tasks, item.task_counts)
        for item in snapshot.capabilities
    ] == [
        (
            "bilibili",
            SourceCapability.SEARCH,
            1,
            {
                status: int(status is OperationalTaskStatus.PARTIALLY_SUCCEEDED)
                for status in OperationalTaskStatus
            },
        ),
        (
            "x",
            SourceCapability.COMMENTS,
            1,
            {
                status: int(status is OperationalTaskStatus.SUCCEEDED)
                for status in OperationalTaskStatus
            },
        ),
        (
            "x",
            SourceCapability.SEARCH,
            2,
            {
                status: int(
                    status in {OperationalTaskStatus.FAILED, OperationalTaskStatus.SUCCEEDED}
                )
                for status in OperationalTaskStatus
            },
        ),
    ]
    assert other_owner.capabilities == ()


def test_continuous_failure_issue_requires_three_terminal_jobs_and_owner_scope(
    observation_context: ObservationTestContext,
) -> None:
    with observation_context.sessions() as session:
        service = JobService(session, clock=lambda: WINDOW_START)
        jobs = [
            service.accept(
                owner_id=observation_context.owner_id,
                command=JobAcceptanceInput(
                    operation_id=uuid4(),
                    kind="monitor.collect",
                    observation=JobObservationContext(
                        configuration_ref="monitor-config-1",
                        configuration_version=version,
                        source_key="x",
                        source_capability=SourceCapability.SEARCH,
                    ),
                    scope={"window": version},
                ),
            )
            for version in range(1, 4)
        ]
        queued = service.accept(
            owner_id=observation_context.owner_id,
            command=JobAcceptanceInput(
                operation_id=uuid4(),
                kind="monitor.collect",
                observation=_observation(version=4),
                scope={"window": 4},
            ),
        )
        running = service.accept(
            owner_id=observation_context.owner_id,
            command=JobAcceptanceInput(
                operation_id=uuid4(),
                kind="monitor.collect",
                observation=_observation(version=5),
                scope={"window": 5},
            ),
        )
        below_threshold = service.accept(
            owner_id=observation_context.owner_id,
            command=JobAcceptanceInput(
                operation_id=uuid4(),
                kind="monitor.collect",
                observation=JobObservationContext(
                    configuration_ref="monitor-config-2",
                    configuration_version=1,
                    source_key="x",
                    source_capability=SourceCapability.SEARCH,
                ),
                scope={"window": 4},
            ),
        )

    with observation_context.engine.begin() as connection:
        for index, job in enumerate([*jobs, below_threshold], start=1):
            completed_at = WINDOW_START + timedelta(minutes=index)
            connection.execute(
                text(
                    "UPDATE jobs SET status = 'failed', started_at = :started_at, "
                    "completed_at = :completed_at, updated_at = :completed_at, "
                    "last_error_code = 'source.timeout', "
                    "last_error_category = 'transient', last_error_at = :completed_at, "
                    "next_action = '检查来源连接', manual_retry_allowed = false "
                    "WHERE id = :job_id"
                ),
                {
                    "started_at": completed_at - timedelta(seconds=1),
                    "completed_at": completed_at,
                    "job_id": job.id,
                },
            )
        connection.execute(
            text(
                "UPDATE jobs SET status = 'running', started_at = :started_at, "
                "lease_owner = 'worker-running', lease_epoch = 1, "
                "lease_expires_at = :lease_expires_at, updated_at = :started_at "
                "WHERE id = :job_id"
            ),
            {
                "started_at": WINDOW_START + timedelta(minutes=5),
                "lease_expires_at": WINDOW_START + timedelta(minutes=6),
                "job_id": running.id,
            },
        )

    with observation_context.sessions() as session:
        service = JobService(session)
        issues = service.list_continuous_failure_issues(owner_id=observation_context.owner_id)
        other_owner_issues = service.list_continuous_failure_issues(
            owner_id=observation_context.other_owner_id
        )

    assert len(issues) == 1
    assert issues[0].source_key == "x"
    assert issues[0].source_capability == SourceCapability.SEARCH
    assert issues[0].configuration_ref == "monitor-config-1"
    assert issues[0].configuration_version == 3
    assert issues[0].latest_failed_job_id == jobs[-1].id
    assert issues[0].failure.error_code == "source.timeout"
    assert issues[0].failure.next_action == "检查来源连接"
    assert queued.status == "queued"
    assert other_owner_issues == ()


def test_success_resets_continuous_failure_issue_without_deleting_history(
    observation_context: ObservationTestContext,
) -> None:
    with observation_context.sessions() as session:
        service = JobService(session, clock=lambda: WINDOW_START)
        jobs = [
            service.accept(
                owner_id=observation_context.owner_id,
                command=_command(version=version),
            )
            for version in range(1, 5)
        ]

    with observation_context.engine.begin() as connection:
        for index, (job, status) in enumerate(
            zip(jobs, ("failed", "failed", "failed", "succeeded"), strict=True),
            start=1,
        ):
            completed_at = WINDOW_START + timedelta(minutes=index)
            failure_values = (
                {
                    "last_error_code": "source.timeout",
                    "last_error_category": "transient",
                    "last_error_at": completed_at,
                    "next_action": "检查来源连接",
                }
                if status == "failed"
                else {
                    "last_error_code": None,
                    "last_error_category": None,
                    "last_error_at": None,
                    "next_action": None,
                }
            )
            connection.execute(
                text(
                    "UPDATE jobs SET status = :status, started_at = :started_at, "
                    "completed_at = :completed_at, updated_at = :completed_at, "
                    "last_error_code = :last_error_code, "
                    "last_error_category = :last_error_category, "
                    "last_error_at = :last_error_at, next_action = :next_action "
                    "WHERE id = :job_id"
                ),
                {
                    "status": status,
                    "started_at": completed_at - timedelta(seconds=1),
                    "completed_at": completed_at,
                    "job_id": job.id,
                    **failure_values,
                },
            )

    with observation_context.sessions() as session:
        issues = JobService(session).list_continuous_failure_issues(
            owner_id=observation_context.owner_id
        )
        history = JobService(session).list_history(
            owner_id=observation_context.owner_id,
            cursor=None,
            limit=10,
        )[0]

    assert issues == ()
    assert {job.id for job in history} == {job.id for job in jobs}


@pytest.mark.parametrize("reset_status", ("partially_succeeded", "cancelled"))
def test_partial_or_cancelled_job_resets_continuous_failure_streak(
    observation_context: ObservationTestContext,
    reset_status: str,
) -> None:
    with observation_context.sessions() as session:
        service = JobService(session, clock=lambda: WINDOW_START)
        jobs = [
            service.accept(
                owner_id=observation_context.owner_id,
                command=_command(version=version),
            )
            for version in range(1, 5)
        ]

    with observation_context.engine.begin() as connection:
        for index, (job, status) in enumerate(
            zip(
                jobs,
                ("failed", "failed", "failed", reset_status),
                strict=True,
            ),
            start=1,
        ):
            completed_at = WINDOW_START + timedelta(minutes=index)
            failure_values = (
                {
                    "last_error_code": "source.timeout",
                    "last_error_category": "transient",
                    "last_error_at": completed_at,
                    "next_action": "检查来源连接",
                }
                if status == "failed"
                else {
                    "last_error_code": None,
                    "last_error_category": None,
                    "last_error_at": None,
                    "next_action": None,
                }
            )
            connection.execute(
                text(
                    "UPDATE jobs SET status = :status, started_at = :started_at, "
                    "completed_at = :completed_at, updated_at = :completed_at, "
                    "last_error_code = :last_error_code, "
                    "last_error_category = :last_error_category, "
                    "last_error_at = :last_error_at, next_action = :next_action "
                    "WHERE id = :job_id"
                ),
                {
                    "status": status,
                    "started_at": completed_at - timedelta(seconds=1),
                    "completed_at": completed_at,
                    "job_id": job.id,
                    **failure_values,
                },
            )

    with observation_context.sessions() as session:
        issues = JobService(session).list_continuous_failure_issues(
            owner_id=observation_context.owner_id
        )

    assert issues == ()
