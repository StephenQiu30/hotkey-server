from __future__ import annotations

import json
import os
import time
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from threading import Barrier, Event
from typing import cast
from uuid import UUID, uuid4

import pytest
from confluent_kafka import Consumer, Message
from confluent_kafka.admin import AdminClient, NewTopic
from redis import Redis
from redis.exceptions import RedisError
from sqlalchemy import Engine, create_engine, event, text
from sqlalchemy.orm import Session, sessionmaker
from structlog.contextvars import get_contextvars

from core.config import Settings
from core.errors import ApplicationError
from jobs.execution import (
    JobCompletion,
    JobExecutionFailure,
    JobExecutionService,
    JobProgress,
    MessageReference,
    StaleExecutionLeaseError,
    plan_catchup_windows,
)
from jobs.schemas import (
    JobAcceptanceInput,
    JobFailureCategory,
    JobObservationContext,
    JobReliabilityOutcome,
    JobStage,
    JobStatus,
)
from jobs.services import JobObservationService, JobService, OutboxService
from sources.contracts import SourceCapability
from worker.app import JobExecutionContext, create_job_message_handler
from worker.messaging import (
    MessageDeferredError,
    create_producer,
    decode_job_message,
    process_message,
    publish_outbox,
)


@dataclass(frozen=True, slots=True)
class JobTestContext:
    engine: Engine
    sessions: sessionmaker[Session]
    owner_id: UUID


@pytest.fixture
def job_context() -> Iterator[JobTestContext]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    engine = create_engine(database_url, pool_size=4)
    sessions = sessionmaker(bind=engine, expire_on_commit=False)
    owner_id = uuid4()
    with engine.begin() as connection:
        connection.execute(
            text(
                "TRUNCATE content_version_relations, content_visibility_observations, "
                "content_observations, content_versions, "
                "content_discoveries, content_threads, content_records, "
                "source_capability_evidence, source_connection_versions, "
                "source_connections, provenance_manifest_inputs, provenance_manifests, "
                "evidence_cleanup_targets, evidence_deletions, "
                "evidence_resources, evidence_retention_policies, source_access_policies, "
                "resource_budget_reservations, resource_budget_windows, "
                "resource_budget_policies, resource_usage_attempts, "
                "resource_component_policies, "
                "job_stage_attempts, processed_messages, job_attempts, "
                "ai_calls, "
                "content_annotations, reports, monitor_schedules, "
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
                "VALUES (:id, 'job-test-owner', 'test-only-hash', 1, now(), now())"
            ),
            {"id": owner_id},
        )
    try:
        yield JobTestContext(engine=engine, sessions=sessions, owner_id=owner_id)
    finally:
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE content_version_relations, content_visibility_observations, "
                    "content_observations, content_versions, "
                    "content_discoveries, content_threads, content_records, "
                    "source_capability_evidence, source_connection_versions, "
                    "source_connections, provenance_manifest_inputs, provenance_manifests, "
                    "evidence_cleanup_targets, evidence_deletions, "
                    "evidence_resources, evidence_retention_policies, source_access_policies, "
                    "resource_budget_reservations, resource_budget_windows, "
                    "resource_budget_policies, resource_usage_attempts, "
                    "resource_component_policies, "
                    "job_stage_attempts, processed_messages, job_attempts, "
                    "ai_calls, "
                    "content_annotations, reports, monitor_schedules, "
                    "outbox_messages, coverage_windows, "
                    "jobs, followed_account_aliases, followed_accounts, "
                    "monitor_topic_versions, monitor_topics, "
                    "identity_sessions, identity_users"
                )
            )
        engine.dispose()


def _command(
    *,
    operation_id: UUID | None = None,
    window: int = 7,
    scheduled_for_at: datetime | None = None,
) -> JobAcceptanceInput:
    return JobAcceptanceInput(
        operation_id=operation_id or uuid4(),
        kind="monitor.collect",
        observation=JobObservationContext(
            configuration_ref="monitor-config-1",
            configuration_version=window,
            source_key="x",
            source_capability=SourceCapability.SEARCH,
        ),
        scheduled_for_at=scheduled_for_at,
        scope={"source_id": "account-1", "window": window},
    )


def _webpage_command(
    *, operation_id: UUID | None = None, scheduled_for_at: datetime | None = None
) -> JobAcceptanceInput:
    return JobAcceptanceInput(
        operation_id=operation_id or uuid4(),
        kind="webpage.collect",
        observation=JobObservationContext(
            configuration_ref="web-collector-1",
            configuration_version=1,
            source_key="web",
            source_capability=SourceCapability.PAGE_CONTENT,
        ),
        scheduled_for_at=scheduled_for_at,
        scope={"target": "https://example.com/"},
    )


def _counts(context: JobTestContext) -> tuple[int, int]:
    with context.engine.connect() as connection:
        jobs = connection.execute(text("SELECT count(*) FROM jobs")).scalar_one()
        outbox = connection.execute(text("SELECT count(*) FROM outbox_messages")).scalar_one()
    return int(jobs), int(outbox)


def _poll_message(consumer: Consumer, timeout_seconds: float = 10) -> Message:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        message = consumer.poll(0.2)
        if message is None:
            continue
        if message.error() is not None:
            raise RuntimeError(str(message.error()))
        return message
    raise AssertionError("Kafka message was not received before the timeout")


def _wait_for_assignment(consumer: Consumer, timeout_seconds: float = 10) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        consumer.poll(0.1)
        if consumer.assignment():
            return
    raise AssertionError("Kafka consumer did not receive a partition assignment")


class StoredOutboxMessage:
    def __init__(
        self,
        *,
        job_id: UUID,
        message_id: UUID,
        event_type: str,
        payload: dict[str, object],
        offset: int,
    ) -> None:
        self._job_id = job_id
        self._message_id = message_id
        self._event_type = event_type
        self._payload = payload
        self._offset = offset

    def topic(self) -> str:
        return "hotkey.jobs.accepted.v2"

    def value(self) -> bytes:
        return json.dumps(
            {
                **self._payload,
                "schema_version": 2 if self._event_type == "job.accepted.v2" else 1,
                "message_id": str(self._message_id),
                "event_type": self._event_type,
            }
        ).encode()

    def key(self) -> bytes:
        return str(self._job_id).encode()

    def partition(self) -> int:
        return 0

    def offset(self) -> int:
        return self._offset

    def headers(self) -> list[tuple[str, bytes]]:
        return [("hotkey-message-id", str(self._message_id).encode())]


def _stored_message(
    context: JobTestContext,
    *,
    job_id: UUID,
    dispatch_sequence: int,
    offset: int,
) -> Message:
    with context.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT id, event_type, payload FROM outbox_messages "
                "WHERE aggregate_id = :job_id AND dispatch_sequence = :dispatch_sequence"
            ),
            {"job_id": job_id, "dispatch_sequence": dispatch_sequence},
        ).one()
    return cast(
        Message,
        StoredOutboxMessage(
            job_id=job_id,
            message_id=row.id,
            event_type=row.event_type,
            payload=row.payload,
            offset=offset,
        ),
    )


def test_lost_response_retry_returns_the_original_job_and_one_outbox(
    job_context: JobTestContext,
) -> None:
    command = _command()
    with job_context.sessions() as session:
        first = JobService(session).accept(owner_id=job_context.owner_id, command=command)
    with job_context.sessions() as session:
        repeated = JobService(session).accept(owner_id=job_context.owner_id, command=command)

    assert repeated == first
    assert first.status == "queued"
    assert first.scheduled_for_at is None
    assert _counts(job_context) == (1, 1)
    with job_context.engine.connect() as connection:
        payload = connection.execute(text("SELECT payload FROM outbox_messages")).scalar_one()
    assert payload == {
        "job_id": str(first.id),
        "kind": command.kind,
        "operation_id": str(command.operation_id),
        "owner_id": str(job_context.owner_id),
        "configuration_ref": "monitor-config-1",
        "configuration_version": 7,
        "source_key": "x",
        "source_capability": "search",
    }


def test_reliability_snapshot_uses_logical_start_and_durable_source_evidence(
    job_context: JobTestContext,
) -> None:
    window_start = datetime(2026, 9, 24, 12, tzinfo=UTC)
    window_end = window_start + timedelta(minutes=40)
    scheduled_start = window_start + timedelta(minutes=1)
    scheduled_job_created_at = window_start + timedelta(minutes=2)
    immediate_created_at = window_start + timedelta(minutes=3)
    success_start = window_start + timedelta(minutes=4)
    late_success_start = window_start + timedelta(minutes=5)
    source_completed_at = scheduled_start + timedelta(minutes=30)
    unattributed_completed_at = immediate_created_at + timedelta(minutes=1)
    success_completed_at = success_start + timedelta(minutes=30)
    late_success_completed_at = late_success_start + timedelta(minutes=30, microseconds=1)
    operation_id = uuid4()
    with job_context.sessions() as session:
        source_job = JobService(
            session,
            clock=lambda: scheduled_job_created_at,
        ).accept(
            owner_id=job_context.owner_id,
            command=_webpage_command(
                operation_id=operation_id,
                scheduled_for_at=scheduled_start,
            ),
        )
    with job_context.sessions() as session:
        unattributed_job = JobService(
            session,
            clock=lambda: immediate_created_at,
        ).accept(
            owner_id=job_context.owner_id,
            command=_webpage_command(),
        )
    with job_context.sessions() as session:
        success_job = JobService(
            session,
            clock=lambda: success_start,
        ).accept(
            owner_id=job_context.owner_id,
            command=_webpage_command(),
        )
    with job_context.sessions() as session:
        late_success_job = JobService(
            session,
            clock=lambda: late_success_start,
        ).accept(
            owner_id=job_context.owner_id,
            command=_webpage_command(),
        )
    with job_context.sessions() as session:
        boundary_job = JobService(
            session,
            clock=lambda: immediate_created_at + timedelta(minutes=1),
        ).accept(
            owner_id=job_context.owner_id,
            command=_webpage_command(scheduled_for_at=window_end - timedelta(minutes=30)),
        )

    with job_context.engine.begin() as connection:
        for job, completed_at, error_code in (
            (source_job, source_completed_at, "source_upstream_unavailable"),
            (unattributed_job, unattributed_completed_at, "job_execution_timeout"),
        ):
            connection.execute(
                text(
                    "UPDATE jobs SET status = 'failed', started_at = :started_at, "
                    "completed_at = :completed_at, updated_at = :completed_at, "
                    "last_error_code = :error_code, last_error_category = 'transient', "
                    "last_error_at = :completed_at, next_action = 'Retry after recovery' "
                    "WHERE id = :job_id"
                ),
                {
                    "started_at": job.scheduled_for_at or job.created_at,
                    "completed_at": completed_at,
                    "error_code": error_code,
                    "job_id": job.id,
                },
            )
        for job, completed_at in (
            (success_job, success_completed_at),
            (late_success_job, late_success_completed_at),
        ):
            connection.execute(
                text(
                    "UPDATE jobs SET status = 'succeeded', started_at = :started_at, "
                    "completed_at = :completed_at, updated_at = :completed_at "
                    "WHERE id = :job_id"
                ),
                {
                    "started_at": job.created_at,
                    "completed_at": completed_at,
                    "job_id": job.id,
                },
            )

        connection_id = uuid4()
        component_policy_id = uuid4()
        attempt_id = uuid4()
        evidence_at = source_completed_at - timedelta(seconds=1)
        connection.execute(
            text(
                "INSERT INTO source_connections "
                "(id, owner_id, source_key, status, current_version, created_at, updated_at) "
                "VALUES (:id, :owner_id, 'web', 'active', 1, :now, :now)"
            ),
            {"id": connection_id, "owner_id": job_context.owner_id, "now": window_start},
        )
        connection.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, auth_kind, secret_ref, config, "
                "created_by, created_at) VALUES (:id, 1, :owner_id, 'none', NULL, '{}'::jsonb, "
                ":owner_id, :now)"
            ),
            {"id": connection_id, "owner_id": job_context.owner_id, "now": window_start},
        )
        connection.execute(
            text(
                "INSERT INTO resource_component_policies "
                "(id, owner_id, component_key, component_version, cost_class, enabled_for_core, "
                "terms_reference, reviewed_at, policy_version, created_at, updated_at) VALUES "
                "(:id, :owner_id, 'collector.firecrawl', '2.11.162', 'zero_price', true, "
                "'test-fixture', :now, 1, :now, :now)"
            ),
            {"id": component_policy_id, "owner_id": job_context.owner_id, "now": window_start},
        )
        connection.execute(
            text(
                "INSERT INTO resource_usage_attempts "
                "(id, owner_id, attempt_id, operation_id, component_policy_id, component_version, "
                "usage_kind, stage, outcome, started_at, finished_at) VALUES "
                "(:id, :owner_id, :attempt_id, :operation_id, :policy_id, '2.11.162', "
                "'collector_call', 'page_content.fetch', 'failed', :started_at, :finished_at)"
            ),
            {
                "id": uuid4(),
                "owner_id": job_context.owner_id,
                "attempt_id": attempt_id,
                "operation_id": source_job.operation_id,
                "policy_id": component_policy_id,
                "started_at": evidence_at - timedelta(seconds=1),
                "finished_at": evidence_at,
            },
        )
        connection.execute(
            text(
                "INSERT INTO source_capability_evidence "
                "(id, operation_id, owner_id, connection_id, connection_version, capability, "
                "entry_point, kind, outcome, stop_reason, resource_ref, component_name, "
                "component_version, observed_at, created_at) VALUES "
                "(:id, :attempt_id, :owner_id, :connection_id, 1, 'page_content', 'manual', "
                "'persisted_read', 'failed', 'upstream_error', NULL, 'firecrawl', '2.11.162', "
                ":now, :now)"
            ),
            {
                "id": uuid4(),
                "attempt_id": attempt_id,
                "owner_id": job_context.owner_id,
                "connection_id": connection_id,
                "now": evidence_at,
            },
        )

    with job_context.sessions() as session:
        snapshot = JobObservationService(session).reliability_snapshot(
            owner_id=job_context.owner_id,
            window_start=window_start,
            window_end=window_end,
        )

    assert snapshot.sla_seconds == 1800
    assert snapshot.total_jobs == 4
    assert snapshot.on_time_jobs == 2
    assert snapshot.on_time_rate == 0.5
    assert boundary_job.id not in {record.job_id for record in snapshot.records}
    assert snapshot.outcome_counts == {
        JobReliabilityOutcome.SUCCEEDED: 2,
        JobReliabilityOutcome.PARTIALLY_SUCCEEDED: 0,
        JobReliabilityOutcome.SOURCE_FAILURE: 1,
        JobReliabilityOutcome.UNATTRIBUTED_FAILURE: 1,
        JobReliabilityOutcome.CANCELLED: 0,
        JobReliabilityOutcome.IN_PROGRESS: 0,
    }
    source_record, unattributed_record, success_record, late_success_record = snapshot.records
    assert source_record.job_id == source_job.id
    assert source_record.logical_start_at == scheduled_start
    assert source_record.sla_deadline_at == source_completed_at
    assert source_record.elapsed_us == 1_800_000_000
    assert source_record.outcome is JobReliabilityOutcome.SOURCE_FAILURE
    assert source_record.failed_source_evidence_count == 1
    assert source_record.on_time is True
    assert unattributed_record.job_id == unattributed_job.id
    assert unattributed_record.logical_start_at == immediate_created_at
    assert unattributed_record.outcome is JobReliabilityOutcome.UNATTRIBUTED_FAILURE
    assert unattributed_record.failed_source_evidence_count == 0
    assert unattributed_record.on_time is False
    assert success_record.job_id == success_job.id
    assert success_record.elapsed_us == 1_800_000_000
    assert success_record.on_time is True
    assert late_success_record.job_id == late_success_job.id
    assert late_success_record.elapsed_us == 1_800_000_001
    assert late_success_record.on_time is False


def test_transient_failure_retries_twice_then_fails_without_replay_duplicates(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(owner_id=job_context.owner_id, command=_command())
    clock = [job.created_at + timedelta(seconds=1)]
    calls: list[int] = []

    def fail_after_request(context: JobExecutionContext) -> None:
        assert context.begin_request() is True
        calls.append(context.lease.epoch)
        raise JobExecutionFailure(
            error_code="source_timeout",
            category=JobFailureCategory.TRANSIENT,
            occurred_at=clock[0],
            next_action="等待来源策略安排的下一次尝试",
            manual_retry_allowed=True,
            retry_at=clock[0] + timedelta(seconds=1),
            max_attempts=3,
        )

    handler = create_job_message_handler(
        job_context.sessions,
        {"monitor.collect": fail_after_request},
        worker_id="worker-transient",
        lease_seconds=60,
        clock=lambda: clock[0],
    )

    first = _stored_message(job_context, job_id=job.id, dispatch_sequence=1, offset=0)
    handler(first)
    handler(first)
    handler(first)
    with job_context.engine.connect() as connection:
        after_replay = connection.execute(
            text(
                "SELECT (SELECT count(*) FROM job_attempts WHERE job_id = :job_id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = :job_id), "
                "(SELECT count(*) FROM outbox_messages WHERE aggregate_id = :job_id)"
            ),
            {"job_id": job.id},
        ).one()
    assert tuple(after_replay) == (1, 1, 2)

    clock[0] += timedelta(seconds=1)
    handler(_stored_message(job_context, job_id=job.id, dispatch_sequence=2, offset=1))
    clock[0] += timedelta(seconds=1)
    handler(_stored_message(job_context, job_id=job.id, dispatch_sequence=3, offset=2))

    with job_context.sessions() as session:
        status = JobService(session, clock=lambda: clock[0]).get_status(
            owner_id=job_context.owner_id,
            job_id=job.id,
        )
    with job_context.engine.connect() as connection:
        outcomes = (
            connection.execute(
                text(
                    "SELECT outcome FROM job_attempts WHERE job_id = :job_id ORDER BY lease_epoch"
                ),
                {"job_id": job.id},
            )
            .scalars()
            .all()
        )
        counts = connection.execute(
            text(
                "SELECT (SELECT count(*) FROM processed_messages WHERE job_id = :job_id), "
                "(SELECT count(*) FROM outbox_messages WHERE aggregate_id = :job_id)"
            ),
            {"job_id": job.id},
        ).one()

    assert calls == [1, 2, 3]
    assert outcomes == ["delayed", "delayed", "failed"]
    assert tuple(counts) == (3, 3)
    assert status.status == "failed"
    assert status.retry_count == 2
    assert status.progress.requests_sent == 3
    assert status.next_run_at is None
    assert status.failure is not None
    assert status.failure.error_code == "source_timeout"
    assert status.failure.category == "transient"


def test_permission_failure_is_terminal_and_never_switches_context(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(owner_id=job_context.owner_id, command=_command())
    clock = [job.created_at + timedelta(seconds=1)]
    seen_sources: list[tuple[str | None, str | None]] = []

    def deny(context: JobExecutionContext) -> None:
        seen_sources.append(
            (
                context.message.source_key,
                context.message.source_capability,
            )
        )
        raise JobExecutionFailure(
            error_code="source_permission_denied",
            category=JobFailureCategory.PERMISSION_DENIED,
            occurred_at=clock[0],
            next_action="检查当前连接的访问权限",
            manual_retry_allowed=True,
        )

    handler = create_job_message_handler(
        job_context.sessions,
        {"monitor.collect": deny},
        worker_id="worker-permission",
        lease_seconds=60,
        clock=lambda: clock[0],
    )
    message = _stored_message(job_context, job_id=job.id, dispatch_sequence=1, offset=0)
    handler(message)
    handler(message)
    handler(message)

    with job_context.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT j.status, j.last_error_category, j.retry_count, "
                "(SELECT count(*) FROM job_attempts WHERE job_id = j.id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = j.id), "
                "(SELECT count(*) FROM outbox_messages WHERE aggregate_id = j.id) "
                "FROM jobs j WHERE j.id = :job_id"
            ),
            {"job_id": job.id},
        ).one()
    assert seen_sources == [("x", SourceCapability.SEARCH)]
    assert tuple(row) == ("failed", "permission_denied", 0, 1, 1, 1)


def test_cancellation_wins_when_handler_failure_finishes_concurrently(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(owner_id=job_context.owner_id, command=_command())
    clock = [job.created_at + timedelta(seconds=1)]
    with job_context.sessions() as session:
        lease = JobExecutionService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).acquire(job_id=job.id, worker_id="worker-cancel-race")
    with job_context.sessions() as session:
        JobService(session, clock=lambda: clock[0]).request_cancel(
            owner_id=job_context.owner_id,
            job_id=job.id,
        )

    with job_context.sessions() as session:
        JobExecutionService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).record_failure(
            lease,
            message=MessageReference(
                message_id=uuid4(),
                topic="hotkey.jobs.accepted.v2",
                partition=0,
                offset=12,
            ),
            failure=JobExecutionFailure(
                error_code="job_execution_timeout",
                category=JobFailureCategory.TRANSIENT,
                occurred_at=clock[0],
                next_action="确认目标服务状态后手动重试",
                manual_retry_allowed=True,
            ),
        )

    with job_context.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT status, retry_count, last_error_code, "
                "(SELECT outcome FROM job_attempts WHERE job_id = jobs.id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = jobs.id), "
                "(SELECT count(*) FROM outbox_messages WHERE aggregate_id = jobs.id) "
                "FROM jobs WHERE id = :job_id"
            ),
            {"job_id": job.id},
        ).one()
    assert tuple(row) == ("cancelled", 0, None, "cancelled", 1, 1)


def test_message_with_live_lease_is_deferred_without_duplicate_execution(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(owner_id=job_context.owner_id, command=_command())
    clock = [job.created_at + timedelta(seconds=1)]
    with job_context.sessions() as session:
        JobExecutionService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).acquire(job_id=job.id, worker_id="worker-owner")
    handler = create_job_message_handler(
        job_context.sessions,
        {"monitor.collect": lambda _context: None},
        worker_id="worker-duplicate",
        lease_seconds=60,
        clock=lambda: clock[0],
    )

    with pytest.raises(MessageDeferredError):
        handler(_stored_message(job_context, job_id=job.id, dispatch_sequence=1, offset=12))

    with job_context.engine.connect() as connection:
        counts = connection.execute(
            text(
                "SELECT (SELECT count(*) FROM job_attempts WHERE job_id = :job_id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = :job_id)"
            ),
            {"job_id": job.id},
        ).one()
    assert tuple(counts) == (1, 0)


def test_retry_outbox_is_published_only_when_due_and_only_once(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(owner_id=job_context.owner_id, command=_command())
    clock = [job.created_at + timedelta(seconds=1)]
    published: list[tuple[str, int]] = []
    with job_context.sessions() as session:
        assert (
            OutboxService(session).publish_pending(
                lambda envelope: published.append((envelope.event_type, envelope.schema_version)),
                published_at=clock[0],
            )
            == 1
        )

    def delay(_context: JobExecutionContext) -> None:
        raise JobExecutionFailure(
            error_code="source_rate_limited",
            category=JobFailureCategory.RATE_LIMITED,
            occurred_at=clock[0],
            next_action="等待来源给出的恢复时间",
            retry_at=clock[0] + timedelta(seconds=10),
            max_attempts=2,
        )

    create_job_message_handler(
        job_context.sessions,
        {"monitor.collect": delay},
        worker_id="worker-rate-limit",
        lease_seconds=60,
        clock=lambda: clock[0],
    )(_stored_message(job_context, job_id=job.id, dispatch_sequence=1, offset=0))

    with job_context.sessions() as session:
        assert (
            OutboxService(session).publish_pending(
                lambda envelope: published.append((envelope.event_type, envelope.schema_version)),
                published_at=clock[0] + timedelta(seconds=9),
            )
            == 0
        )
    with job_context.sessions() as session:
        assert (
            OutboxService(session).publish_pending(
                lambda envelope: published.append((envelope.event_type, envelope.schema_version)),
                published_at=clock[0] + timedelta(seconds=10),
            )
            == 1
        )
    with job_context.sessions() as session:
        assert (
            OutboxService(session).publish_pending(
                lambda envelope: published.append((envelope.event_type, envelope.schema_version)),
                published_at=clock[0] + timedelta(seconds=11),
            )
            == 0
        )
    assert published == [("job.accepted.v2", 2), ("job.retry_scheduled.v1", 1)]


def test_reused_operation_id_with_different_scope_is_rejected(
    job_context: JobTestContext,
) -> None:
    operation_id = uuid4()
    with job_context.sessions() as session:
        original = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(operation_id=operation_id, window=7),
        )
    with (
        job_context.sessions() as session,
        pytest.raises(ApplicationError, match="idempotency_conflict") as captured,
    ):
        JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(operation_id=operation_id, window=8),
        )

    assert captured.value.code == "idempotency_conflict"
    assert _counts(job_context) == (1, 1)
    with job_context.engine.connect() as connection:
        stored_scope = connection.execute(
            text("SELECT scope FROM jobs WHERE id = :id"), {"id": original.id}
        ).scalar_one()
    assert stored_scope["window"] == 7


def test_transaction_failure_creates_neither_job_nor_outbox(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:

        def fail_after_flush(_: Session, __: object) -> None:
            raise RuntimeError("injected failure after transaction flush")

        event.listen(session, "after_flush_postexec", fail_after_flush)
        try:
            with pytest.raises(RuntimeError, match="injected failure"):
                JobService(session).accept(
                    owner_id=job_context.owner_id,
                    command=_command(),
                )
        finally:
            event.remove(session, "after_flush_postexec", fail_after_flush)

    assert _counts(job_context) == (0, 0)


def test_concurrent_retries_return_one_job_and_one_outbox(
    job_context: JobTestContext,
) -> None:
    command = _command()
    barrier = Barrier(2)

    def accept() -> UUID:
        with job_context.sessions() as session:
            barrier.wait(timeout=5)
            return (
                JobService(session)
                .accept(
                    owner_id=job_context.owner_id,
                    command=command,
                )
                .id
            )

    with ThreadPoolExecutor(max_workers=2) as executor:
        first = executor.submit(accept)
        second = executor.submit(accept)

    assert first.result() == second.result()
    assert _counts(job_context) == (1, 1)


def test_expired_lease_recovers_checkpoint_and_fences_old_worker(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(),
        )
    clock = [job.created_at + timedelta(seconds=1)]
    with job_context.sessions() as session:
        service = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0])
        old = service.acquire(job_id=job.id, worker_id="worker-old")
        old = service.save_checkpoint(old, sequence=1, checkpoint={"page": 1})

    clock[0] += timedelta(seconds=61)
    with job_context.sessions() as session:
        service = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0])
        recovered = service.acquire(job_id=job.id, worker_id="worker-new")

    assert recovered.epoch == old.epoch + 1
    assert recovered.checkpoint_sequence == 1
    assert recovered.checkpoint == {"page": 1}

    with job_context.sessions() as session:
        service = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0])
        with pytest.raises(StaleExecutionLeaseError):
            service.save_checkpoint(old, sequence=2, checkpoint={"page": 2})

    reference = MessageReference(
        message_id=uuid4(),
        topic="hotkey.tests.jobs",
        partition=0,
        offset=0,
    )
    with job_context.sessions() as session:
        service = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0])
        recovered = service.save_checkpoint(
            recovered,
            sequence=2,
            checkpoint={"page": 2},
        )
        service.complete(recovered, message=reference)

    with job_context.engine.connect() as connection:
        job_row = connection.execute(
            text(
                "SELECT status, checkpoint_sequence, checkpoint, lease_owner "
                "FROM jobs WHERE id = :id"
            ),
            {"id": job.id},
        ).one()
        attempts = connection.execute(
            text(
                "SELECT lease_epoch, outcome FROM job_attempts "
                "WHERE job_id = :id ORDER BY lease_epoch"
            ),
            {"id": job.id},
        ).all()

    assert tuple(job_row) == ("succeeded", 2, {"page": 2}, None)
    assert attempts == [(1, "expired"), (2, "succeeded")]


def test_cancelled_inflight_response_is_saved_without_starting_next_request(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(),
        )
    with job_context.engine.connect() as connection:
        outbox = connection.execute(
            text("SELECT id, payload FROM outbox_messages WHERE aggregate_id = :job_id"),
            {"job_id": job.id},
        ).one()

    class FakeMessage:
        def topic(self) -> str:
            return "hotkey.jobs.accepted.v2"

        def value(self) -> bytes:
            return json.dumps(
                {
                    **outbox.payload,
                    "schema_version": 2,
                    "message_id": str(outbox.id),
                    "event_type": "job.accepted.v2",
                }
            ).encode()

        def key(self) -> bytes:
            return str(job.id).encode()

        def partition(self) -> int:
            return 0

        def offset(self) -> int:
            return 0

        def headers(self) -> list[tuple[str, bytes]]:
            return [("hotkey-message-id", str(outbox.id).encode())]

    first_request_started = Event()
    cancel_persisted = Event()
    requests: list[str] = []
    renewed_deadlines: list[datetime] = []

    def collect(context: JobExecutionContext) -> None:
        assert context.begin_request() is True
        requests.append("request-1")
        first_request_started.set()
        assert cancel_persisted.wait(timeout=5)
        context.save_checkpoint(
            1,
            {"cursor": "after-1"},
            progress=JobProgress(
                stage=JobStage.SAVE,
                items_saved=3,
            ),
        )
        renewed_deadlines.append(context.lease.expires_at)
        if context.begin_request():
            requests.append("request-2")

    message_handler = create_job_message_handler(
        job_context.sessions,
        {"monitor.collect": collect},
        worker_id="worker-cancel-test",
        lease_seconds=60,
    )

    with ThreadPoolExecutor(max_workers=1) as executor:
        completed = executor.submit(message_handler, cast(Message, FakeMessage()))
        assert first_request_started.wait(timeout=5)
        with job_context.sessions() as session:
            cancelling = JobService(session).request_cancel(
                owner_id=job_context.owner_id,
                job_id=job.id,
            )
        assert cancelling.status == "cancelling"
        assert cancelling.progress.requests_sent == 1
        assert cancelling.cancellation is not None
        assert cancelling.cancellation.deadline_at is not None
        cancel_persisted.set()
        completed.result(timeout=5)

    with job_context.sessions() as session:
        finished = JobService(session).get_status(
            owner_id=job_context.owner_id,
            job_id=job.id,
        )
    with job_context.engine.connect() as connection:
        stored = connection.execute(
            text(
                "SELECT status, checkpoint_sequence, checkpoint, requests_sent, items_saved, "
                "progress_stage, cancel_deadline_at, lease_owner, "
                "(SELECT outcome FROM job_attempts WHERE job_id = jobs.id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = jobs.id) "
                "FROM jobs WHERE id = :job_id"
            ),
            {"job_id": job.id},
        ).one()

    assert requests == ["request-1"]
    assert renewed_deadlines == [cancelling.cancellation.deadline_at]
    assert finished.status == "cancelled"
    assert finished.progress.requests_sent == 1
    assert finished.progress.items_saved == 3
    assert tuple(stored) == (
        "cancelled",
        1,
        {"cursor": "after-1"},
        1,
        3,
        "save",
        cancelling.cancellation.deadline_at,
        None,
        "cancelled",
        1,
    )


def test_cancellation_timeout_remains_visible_for_review(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(),
        )
    clock = [job.created_at + timedelta(seconds=1)]
    with job_context.sessions() as session:
        JobExecutionService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).acquire(job_id=job.id, worker_id="worker-timeout")

    clock[0] += timedelta(seconds=10)
    with job_context.sessions() as session:
        cancelling = JobService(session, clock=lambda: clock[0]).request_cancel(
            owner_id=job_context.owner_id,
            job_id=job.id,
        )
    assert cancelling.cancellation is not None
    assert cancelling.cancellation.timed_out is False

    clock[0] += timedelta(seconds=51)
    with job_context.sessions() as session:
        timed_out = JobService(session, clock=lambda: clock[0]).get_status(
            owner_id=job_context.owner_id,
            job_id=job.id,
        )
    assert timed_out.status == "cancelling"
    assert timed_out.cancellation is not None
    assert timed_out.cancellation.timed_out is True


def test_queued_cancellation_acknowledges_outbox_without_running_handler(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(),
        )
    with job_context.sessions() as session:
        cancelled = JobService(session).request_cancel(
            owner_id=job_context.owner_id,
            job_id=job.id,
        )
    assert cancelled.status == "cancelled"

    with job_context.engine.connect() as connection:
        outbox = connection.execute(
            text("SELECT id, payload FROM outbox_messages WHERE aggregate_id = :job_id"),
            {"job_id": job.id},
        ).one()

    class FakeMessage:
        def topic(self) -> str:
            return "hotkey.jobs.accepted.v2"

        def value(self) -> bytes:
            return json.dumps(
                {
                    **outbox.payload,
                    "schema_version": 2,
                    "message_id": str(outbox.id),
                    "event_type": "job.accepted.v2",
                }
            ).encode()

        def key(self) -> bytes:
            return str(job.id).encode()

        def partition(self) -> int:
            return 0

        def offset(self) -> int:
            return 7

        def headers(self) -> list[tuple[str, bytes]]:
            return [("hotkey-message-id", str(outbox.id).encode())]

    handler_calls: list[UUID] = []
    message_handler = create_job_message_handler(
        job_context.sessions,
        {"monitor.collect": lambda context: handler_calls.append(context.message.job_id)},
        worker_id="worker-queued-cancel",
        lease_seconds=60,
    )

    message_handler(cast(Message, FakeMessage()))
    message_handler(cast(Message, FakeMessage()))

    with job_context.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT j.status, "
                "(SELECT count(*) FROM job_attempts WHERE job_id = j.id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = j.id) "
                "FROM jobs j WHERE j.id = :job_id"
            ),
            {"job_id": job.id},
        ).one()
    assert handler_calls == []
    assert tuple(row) == ("cancelled", 0, 1)


def test_unregistered_historical_job_kind_is_persistently_failed_and_acknowledged(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command().model_copy(update={"kind": "legacy.collect"}),
        )
    message = _stored_message(job_context, job_id=job.id, dispatch_sequence=1, offset=8)
    handler = create_job_message_handler(
        job_context.sessions,
        {},
        worker_id="worker-unregistered-kind",
        lease_seconds=60,
    )

    handler(message)
    handler(message)

    with job_context.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT j.status, j.last_error_code, j.last_error_category, "
                "j.manual_retry_allowed, "
                "(SELECT outcome FROM job_attempts WHERE job_id = j.id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = j.id) "
                "FROM jobs j WHERE j.id = :job_id"
            ),
            {"job_id": job.id},
        ).one()
    assert tuple(row) == (
        "failed",
        "job_handler_unavailable",
        "configuration_unavailable",
        True,
        "failed",
        1,
    )


def test_typed_partial_completion_is_not_recorded_as_full_success(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(),
        )
    clock = [job.created_at + timedelta(seconds=1)]

    def save_partial(context: JobExecutionContext) -> JobCompletion:
        context.save_checkpoint(
            1,
            {"page": 1},
            progress=JobProgress(stage=JobStage.SAVE, items_saved=1),
        )
        return JobCompletion(
            status=JobStatus.PARTIALLY_SUCCEEDED,
            failure=JobExecutionFailure(
                error_code="source_page_incomplete",
                category=JobFailureCategory.PARSE_ERROR,
                occurred_at=clock[0],
                next_action="检查缺失页面后手动重试",
                manual_retry_allowed=True,
            ),
        )

    create_job_message_handler(
        job_context.sessions,
        {"monitor.collect": save_partial},
        worker_id="worker-partial",
        lease_seconds=60,
        clock=lambda: clock[0],
    )(_stored_message(job_context, job_id=job.id, dispatch_sequence=1, offset=9))

    with job_context.sessions() as session:
        status = JobService(session, clock=lambda: clock[0]).get_status(
            owner_id=job_context.owner_id,
            job_id=job.id,
        )
    assert status.status == "partially_succeeded"
    assert status.progress.items_saved == 1
    assert status.failure is not None
    assert status.failure.error_code == "source_page_incomplete"
    assert status.failure.manual_retry_allowed
    with job_context.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT (SELECT outcome FROM job_attempts WHERE job_id = :job_id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = :job_id)"
            ),
            {"job_id": job.id},
        ).one()
    assert tuple(row) == ("succeeded", 1)


def test_concurrent_schedule_window_acceptance_creates_one_job(
    job_context: JobTestContext,
) -> None:
    start = datetime(2026, 9, 21, 0, tzinfo=UTC)
    window = plan_catchup_windows(
        due_from=start,
        due_until=start + timedelta(hours=1),
        cadence=timedelta(hours=1),
        max_windows=3,
    ).windows[0]
    barrier = Barrier(2)

    def accept() -> UUID:
        with job_context.sessions() as session:
            barrier.wait(timeout=5)
            return (
                JobService(session)
                .accept_schedule_window(
                    owner_id=job_context.owner_id,
                    kind="monitor.collect",
                    schedule_key="topic-1",
                    window=window,
                    observation=JobObservationContext(
                        configuration_ref="monitor-config-1",
                        configuration_version=1,
                        source_key="x",
                        source_capability=SourceCapability.SEARCH,
                    ),
                    scope={"source_id": "account-1"},
                )
                .id
            )

    with ThreadPoolExecutor(max_workers=2) as executor:
        first = executor.submit(accept)
        second = executor.submit(accept)

    assert first.result() == second.result()
    assert _counts(job_context) == (1, 1)
    with job_context.engine.connect() as connection:
        scheduled_for_at = connection.execute(
            text("SELECT scheduled_for_at FROM jobs")
        ).scalar_one()
    assert scheduled_for_at == window.end


def test_terminal_failure_does_not_block_later_kafka_job(
    job_context: JobTestContext,
) -> None:
    bootstrap_servers = os.getenv("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS")
    if bootstrap_servers is None:
        pytest.skip("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS is required for Kafka integration")

    topic = f"hotkey.tests.jobs.{uuid4().hex}"
    group_id = f"hotkey-tests-{uuid4().hex}"
    admin = AdminClient({"bootstrap.servers": bootstrap_servers})
    admin.create_topics([NewTopic(topic, num_partitions=1, replication_factor=1)])[topic].result(10)
    consumer = Consumer(
        {
            "bootstrap.servers": bootstrap_servers,
            "group.id": group_id,
            "enable.auto.commit": False,
            "enable.auto.offset.store": False,
            "auto.offset.reset": "earliest",
        }
    )
    consumer.subscribe([topic])

    try:
        _wait_for_assignment(consumer)
        with job_context.sessions() as session:
            failed_job = JobService(session).accept(
                owner_id=job_context.owner_id,
                command=_command(window=1),
            )
        with job_context.engine.begin() as connection:
            connection.execute(
                text("UPDATE outbox_messages SET topic = :topic WHERE aggregate_id = :job_id"),
                {"topic": topic, "job_id": failed_job.id},
            )

        settings = Settings(
            database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
            kafka_bootstrap_servers=bootstrap_servers,
            kafka_delivery_timeout_seconds=10,
        )
        producer = create_producer(settings)

        def publish(envelope) -> None:
            publish_outbox(producer, envelope, timeout_seconds=10)

        with job_context.sessions() as session:
            assert OutboxService(session).publish_pending(publish) == 1

        with job_context.sessions() as session:
            succeeding_job = JobService(session).accept(
                owner_id=job_context.owner_id,
                command=_command(window=2),
            )
        with job_context.engine.begin() as connection:
            connection.execute(
                text("UPDATE outbox_messages SET topic = :topic WHERE aggregate_id = :job_id"),
                {"topic": topic, "job_id": succeeding_job.id},
            )
        with job_context.sessions() as session:
            assert OutboxService(session).publish_pending(publish) == 1

        handled_job_ids: list[UUID] = []

        def handle(context: JobExecutionContext) -> None:
            handled_job_ids.append(context.message.job_id)
            if context.message.job_id == failed_job.id:
                raise JobExecutionFailure(
                    error_code="source_permission_denied",
                    category=JobFailureCategory.PERMISSION_DENIED,
                    occurred_at=datetime.now(UTC),
                    next_action="检查当前连接的访问权限",
                    manual_retry_allowed=True,
                )

        handler = create_job_message_handler(
            job_context.sessions,
            {"monitor.collect": handle},
            worker_id="worker-terminal-failure-isolation",
            lease_seconds=60,
        )
        topic_handlers = {topic: handler}

        first_message = _poll_message(consumer)
        assert decode_job_message(first_message)[0].job_id == failed_job.id
        process_message(consumer, first_message, topic_handlers)

        second_message = _poll_message(consumer)
        assert decode_job_message(second_message)[0].job_id == succeeding_job.id
        process_message(consumer, second_message, topic_handlers)

        with job_context.engine.connect() as connection:
            rows = connection.execute(
                text(
                    "SELECT id, status, "
                    "(SELECT count(*) FROM processed_messages WHERE job_id = jobs.id) "
                    "FROM jobs WHERE id IN (:failed_job_id, :succeeding_job_id)"
                ),
                {
                    "failed_job_id": failed_job.id,
                    "succeeding_job_id": succeeding_job.id,
                },
            ).all()

        assert handled_job_ids == [failed_job.id, succeeding_job.id]
        assert {row.id: (row.status, row[2]) for row in rows} == {
            failed_job.id: ("failed", 1),
            succeeding_job.id: ("succeeded", 1),
        }
    finally:
        consumer.close()
        admin.delete_topics([topic])[topic].result(10)


def test_real_kafka_redelivery_rebalance_and_redis_loss_recover_once(
    job_context: JobTestContext,
) -> None:
    bootstrap_servers = os.getenv("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS")
    if bootstrap_servers is None:
        pytest.skip("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS is required for Kafka integration")

    topic = f"hotkey.tests.jobs.{uuid4().hex}"
    group_id = f"hotkey-tests-{uuid4().hex}"
    admin = AdminClient({"bootstrap.servers": bootstrap_servers})
    admin.create_topics([NewTopic(topic, num_partitions=1, replication_factor=1)])[topic].result(10)

    consumer_config = {
        "bootstrap.servers": bootstrap_servers,
        "group.id": group_id,
        "enable.auto.commit": False,
        "enable.auto.offset.store": False,
        "auto.offset.reset": "earliest",
        "session.timeout.ms": 6_000,
        "max.poll.interval.ms": 120_000,
    }
    first_consumer: Consumer | None = Consumer(consumer_config)
    second_consumer: Consumer | None = None
    try:
        first_consumer.subscribe([topic])
        _wait_for_assignment(first_consumer)

        with job_context.sessions() as session:
            job = JobService(session).accept(
                owner_id=job_context.owner_id,
                command=_command(),
            )
        with job_context.engine.begin() as connection:
            connection.execute(
                text("UPDATE outbox_messages SET topic = :topic WHERE aggregate_id = :job_id"),
                {"topic": topic, "job_id": job.id},
            )

        settings = Settings(
            database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
            kafka_bootstrap_servers=bootstrap_servers,
            kafka_delivery_timeout_seconds=10,
        )
        producer = create_producer(settings)

        def publish_then_lose_database_ack(envelope) -> None:
            publish_outbox(producer, envelope, timeout_seconds=10)
            raise RuntimeError("injected failure after Kafka delivery")

        with (
            job_context.sessions() as session,
            pytest.raises(RuntimeError, match="after Kafka delivery"),
        ):
            OutboxService(session).publish_pending(publish_then_lose_database_ack)

        first_message = _poll_message(first_consumer)
        first_body, first_reference = decode_job_message(first_message)
        assert first_body.job_id == job.id

        clock = [job.created_at + timedelta(seconds=1)]
        interrupted_leases = []

        def interrupt(context: JobExecutionContext) -> None:
            assert get_contextvars() == {
                "configuration_version": 7,
                "job_id": str(job.id),
                "job_kind": "monitor.collect",
                "operation_id": str(job.operation_id),
                "source_capability": "search",
                "source_key": "x",
            }
            context.save_checkpoint(1, {"page": 1})
            interrupted_leases.append(context.lease)
            raise RuntimeError("injected worker interruption")

        interrupted_handler = create_job_message_handler(
            job_context.sessions,
            {"monitor.collect": interrupt},
            worker_id="worker-before-rebalance",
            lease_seconds=60,
            clock=lambda: clock[0],
        )
        with pytest.raises(RuntimeError, match="worker interruption"):
            interrupted_handler(first_message)
        assert get_contextvars() == {}
        old = interrupted_leases[0]

        first_consumer.close()
        first_consumer = None
        with job_context.sessions() as session:
            assert (
                OutboxService(session).publish_pending(
                    lambda envelope: publish_outbox(producer, envelope, timeout_seconds=10)
                )
                == 1
            )

        unreachable_redis = Redis.from_url(
            "redis://127.0.0.1:1/0",
            socket_connect_timeout=0.1,
            socket_timeout=0.1,
        )
        with pytest.raises(RedisError):
            unreachable_redis.ping()

        clock[0] += timedelta(seconds=61)
        second_consumer = Consumer(consumer_config)
        second_consumer.subscribe([topic])
        replayed_message = _poll_message(second_consumer)
        replayed_body, replayed_reference = decode_job_message(replayed_message)
        assert replayed_body.message_id == first_body.message_id
        assert replayed_reference.offset == first_reference.offset

        recovered_calls = []

        def recover(context: JobExecutionContext) -> None:
            assert get_contextvars() == {
                "configuration_version": 7,
                "job_id": str(job.id),
                "job_kind": "monitor.collect",
                "operation_id": str(job.operation_id),
                "source_capability": "search",
                "source_key": "x",
            }
            recovered_calls.append(context.lease.epoch)
            assert context.lease.checkpoint == {"page": 1}
            with (
                pytest.raises(StaleExecutionLeaseError),
                job_context.sessions() as session,
            ):
                JobExecutionService(
                    session,
                    lease_seconds=60,
                    clock=lambda: clock[0],
                ).save_checkpoint(old, sequence=2, checkpoint={"page": 2})
            context.save_checkpoint(2, {"page": 2})

        recovered_handler = create_job_message_handler(
            job_context.sessions,
            {"monitor.collect": recover},
            worker_id="worker-after-rebalance",
            lease_seconds=60,
            clock=lambda: clock[0],
        )
        recovered_handler(replayed_message)
        assert get_contextvars() == {}
        assert recovered_calls == [old.epoch + 1]
        second_consumer.commit(message=replayed_message, asynchronous=False)

        duplicate_message = _poll_message(second_consumer)
        duplicate_body, _duplicate_reference = decode_job_message(duplicate_message)
        assert duplicate_body.message_id == replayed_body.message_id
        recovered_handler(duplicate_message)
        assert recovered_calls == [old.epoch + 1]
        second_consumer.commit(message=duplicate_message, asynchronous=False)

        with job_context.engine.connect() as connection:
            row = connection.execute(
                text(
                    "SELECT j.status, j.checkpoint_sequence, "
                    "(SELECT count(*) FROM processed_messages WHERE job_id = j.id), "
                    "(SELECT count(*) FROM job_attempts WHERE job_id = j.id) "
                    "FROM jobs j WHERE j.id = :job_id"
                ),
                {"job_id": job.id},
            ).one()
            published_at = connection.execute(
                text("SELECT published_at FROM outbox_messages WHERE aggregate_id = :job_id"),
                {"job_id": job.id},
            ).scalar_one()

        assert tuple(row) == ("succeeded", 2, 1, 2)
        assert published_at is not None
    finally:
        if first_consumer is not None:
            first_consumer.close()
        if second_consumer is not None:
            second_consumer.close()
        admin.delete_topics([topic])[topic].result(10)
