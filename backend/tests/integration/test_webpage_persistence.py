from __future__ import annotations

import hashlib
import json
import os
import time
from collections.abc import Iterator
from contextlib import suppress
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from threading import Event
from typing import cast
from uuid import UUID, uuid4

import pytest
from confluent_kafka import Consumer, Message
from confluent_kafka.admin import AdminClient, NewTopic
from sqlalchemy import Engine, create_engine, text
from sqlalchemy.orm import Session, sessionmaker

from connections.schemas import SourceEntryPoint
from content.collection import (
    WebPageBudgetDelayedError,
    WebPageCollectionExecutor,
    WebPageCollectionService,
    WebPageCommitService,
    WebPageFetchInput,
    WebPageFetchService,
)
from content.schemas import PersistContentDocumentInput
from core.config import Settings
from core.errors import ApplicationError
from evidence.schemas import AdmittedSourcePayload, DataClass
from evidence.services import ResourceUnavailableError
from jobs.execution import (
    JobExecutionFailure,
    JobExecutionService,
    JobLeaseUnavailableError,
    StaleExecutionLeaseError,
    resource_attempt_id,
)
from jobs.schemas import (
    BudgetContext,
    BudgetMetric,
    BudgetPolicyInput,
    BudgetReservationInput,
    BudgetScopeKind,
    ComponentPolicyInput,
    CostClass,
    JobFailureCategory,
)
from jobs.services import JobService, OutboxService, ResourceBudgetService, UsageConflictError
from sources.adapters.firecrawl import FirecrawlAdapter
from sources.contracts import (
    SourceCapability,
    SourceDocument,
    SourceStopReason,
    WebPageRequest,
    WebPageResult,
)
from worker.app import (
    ChildJobFailure,
    ChildJobResult,
    JobExecutionContext,
    _finalize_supervised_result,
    create_job_message_handler,
)
from worker.execution import JobProcessSupervisor
from worker.messaging import create_producer, decode_job_message, publish_outbox

_TABLES = (
    "content_version_relations, content_visibility_observations, content_observations, "
    "content_versions, content_discoveries, content_records, source_capability_evidence, "
    "source_connection_versions, source_connections, provenance_manifest_inputs, "
    "provenance_manifests, evidence_cleanup_targets, evidence_deletions, "
    "evidence_resources, evidence_retention_policies, source_access_policies, "
    "resource_budget_reservations, resource_budget_windows, resource_budget_policies, "
    "resource_usage_attempts, resource_component_policies, job_stage_attempts, "
    "processed_messages, job_attempts, "
    "outbox_messages, coverage_windows, "
    "jobs, monitor_topic_versions, "
    "monitor_topics, identity_sessions, identity_users"
)


@dataclass(frozen=True, slots=True)
class WebPageContext:
    engine: Engine
    sessions: sessionmaker[Session]
    owner_id: UUID
    connection_id: UUID
    policy_id: UUID
    retention_id: UUID
    job_id: UUID
    operation_id: UUID
    now: datetime


@pytest.fixture
def webpage_context() -> Iterator[WebPageContext]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")
    engine = create_engine(database_url)
    sessions = sessionmaker(bind=engine, expire_on_commit=False)
    owner_id = uuid4()
    connection_id = uuid4()
    policy_id = uuid4()
    retention_id = uuid4()
    job_id = uuid4()
    operation_id = uuid4()
    now = datetime.now(UTC).replace(microsecond=0)
    field_purposes = {
        "object_type": "资料类型",
        "request_url": "请求来源",
        "final_url": "最终来源",
        "published_at": "来源发布时间",
        "text_scope": "正文完整度",
        "text_origin": "正文来源",
        "text_origin_ref": "提取器版本",
        "title": "资料标题",
        "body": "资料正文",
        "truncation_reason": "截断原因",
    }
    with engine.begin() as connection:
        connection.execute(text(f"TRUNCATE {_TABLES}"))
        connection.execute(text("SET CONSTRAINTS ALL DEFERRED"))
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:id, 'webpage-owner', 'test-only-hash', 1, :now, :now)"
            ),
            {"id": owner_id, "now": now},
        )
        connection.execute(
            text(
                "INSERT INTO source_connections "
                "(id, owner_id, source_key, status, current_version, created_at, updated_at) "
                "VALUES (:id, :owner_id, 'web', 'active', 1, :now, :now)"
            ),
            {"id": connection_id, "owner_id": owner_id, "now": now},
        )
        connection.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, auth_kind, secret_ref, configuration, "
                "created_by, created_at) VALUES "
                "(:id, 1, :owner_id, 'none', NULL, "
                "CAST(:configuration AS jsonb), :owner_id, :now)"
            ),
            {
                "id": connection_id,
                "owner_id": owner_id,
                "configuration": json.dumps({"allowed_hosts": ["example.com"]}),
                "now": now,
            },
        )
        connection.execute(
            text(
                "INSERT INTO source_access_policies "
                "(id, owner_id, source_key, capability, status, enabled, access_basis, "
                "terms_reference, processing_purpose, component_name, component_version, "
                "component_license, field_purposes, reviewed_at, policy_version, "
                "created_at, updated_at) VALUES "
                "(:id, :owner_id, 'web', 'page_content', 'approved', true, 'public_web', "
                "'https://example.com/terms', '公开网页资料采集', 'firecrawl', "
                "'2.11.162', 'AGPL-3.0', CAST(:fields AS jsonb), :now, 1, :now, :now)"
            ),
            {
                "id": policy_id,
                "owner_id": owner_id,
                "fields": json.dumps(field_purposes, ensure_ascii=False),
                "now": now,
            },
        )
        connection.execute(
            text(
                "INSERT INTO evidence_retention_policies "
                "(id, owner_id, source_policy_id, source_policy_version, data_class, "
                "requested_days, source_max_days, effective_days, policy_version, "
                "created_at, updated_at) VALUES "
                "(:id, :owner_id, :policy_id, 1, 'structured', 30, NULL, 30, 1, :now, :now)"
            ),
            {
                "id": retention_id,
                "owner_id": owner_id,
                "policy_id": policy_id,
                "now": now,
            },
        )
        connection.execute(
            text(
                "INSERT INTO jobs "
                "(id, owner_id, operation_id, kind, configuration_ref, "
                "configuration_version, source_key, source_capability, scope, "
                "request_fingerprint, status, created_at, updated_at) VALUES "
                "(:id, :owner_id, :operation_id, 'webpage.collect', 'web:manual', "
                "1, 'web', 'page_content', '{}'::jsonb, :fingerprint, 'queued', :now, :now)"
            ),
            {
                "id": job_id,
                "owner_id": owner_id,
                "operation_id": operation_id,
                "fingerprint": b"w" * 32,
                "now": now,
            },
        )
    try:
        yield WebPageContext(
            engine=engine,
            sessions=sessions,
            owner_id=owner_id,
            connection_id=connection_id,
            policy_id=policy_id,
            retention_id=retention_id,
            job_id=job_id,
            operation_id=operation_id,
            now=now,
        )
    finally:
        with engine.begin() as connection:
            connection.execute(text(f"TRUNCATE {_TABLES}"))
        engine.dispose()


def _command(
    context: WebPageContext, *, operation_id: UUID | None = None
) -> PersistContentDocumentInput:
    collected_at = context.now + timedelta(seconds=1)
    return PersistContentDocumentInput(
        job_id=context.job_id,
        source_operation_id=operation_id or uuid4(),
        connection_id=context.connection_id,
        connection_version=1,
        entry_point=SourceEntryPoint.MANUAL,
        component_name="firecrawl",
        component_version="2.11.162",
        admission=AdmittedSourcePayload(
            policy_id=context.policy_id,
            policy_version=1,
            owner_id=context.owner_id,
            source_key="web",
            capability=SourceCapability.PAGE_CONTENT,
            retention_policy_id=context.retention_id,
            retention_policy_version=1,
            data_class=DataClass.STRUCTURED,
            collected_at=collected_at,
            expires_at=collected_at + timedelta(days=30),
            fields={
                "object_type": "webpage",
                "request_url": "https://example.com/Articles/One?q=1#ignored",
                "final_url": "https://example.com/articles/final?q=1",
                "published_at": "2026-09-22T10:00:00Z",
                "text_scope": "full",
                "text_origin": "machine_extracted",
                "text_origin_ref": "firecrawl/2.11.162",
                "title": "A public page",
                "body": "Persisted Markdown body.",
                "truncation_reason": None,
            },
        ),
    )


def _content_side_effect_counts(context: WebPageContext) -> tuple[int, ...]:
    with context.engine.connect() as connection:
        return tuple(
            int(value)
            for value in connection.execute(
                text(
                    "SELECT (SELECT count(*) FROM content_records), "
                    "(SELECT count(*) FROM content_versions), "
                    "(SELECT count(*) FROM content_observations), "
                    "(SELECT count(*) FROM content_discoveries), "
                    "(SELECT count(*) FROM content_visibility_observations), "
                    "(SELECT count(*) FROM evidence_resources), "
                    "(SELECT count(*) FROM source_capability_evidence)"
                )
            ).one()
        )


def _enable_firecrawl_budget(context: WebPageContext, *, limit_units: int = 2) -> None:
    clock = context.now + timedelta(seconds=1)
    with context.sessions() as session:
        service = ResourceBudgetService(session, clock=lambda: clock)
        service.save_component_policy(
            owner_id=context.owner_id,
            command=ComponentPolicyInput(
                component_key="collector.firecrawl",
                component_version="2.11.162",
                cost_class=CostClass.LOCAL,
                enabled_for_core=True,
                terms_reference="https://github.com/firecrawl/firecrawl",
                reviewed_at=context.now,
            ),
        )
        service.save_budget_policy(
            owner_id=context.owner_id,
            command=BudgetPolicyInput(
                budget_key="global.collector-calls",
                metric=BudgetMetric.COLLECTOR_CALL,
                scope_kind=BudgetScopeKind.GLOBAL,
                scope_reference=None,
                limit_units=limit_units,
                window_seconds=60,
                window_anchor_at=context.now,
                enabled=True,
            ),
        )


class InspectingDocumentAdapter:
    def __init__(self, context: WebPageContext) -> None:
        self._context = context
        self.requests: list[WebPageRequest] = []

    def __enter__(self) -> InspectingDocumentAdapter:
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def fetch_document(self, request: WebPageRequest) -> WebPageResult:
        self.requests.append(request)
        with self._context.engine.connect() as connection:
            preflight = connection.execute(
                text(
                    "SELECT r.status, a.outcome, j.requests_sent "
                    "FROM resource_budget_reservations r "
                    "JOIN resource_usage_attempts a ON a.attempt_id = r.reservation_id "
                    "JOIN jobs j ON j.id = :job_id"
                ),
                {"job_id": self._context.job_id},
            ).one()
        assert tuple(preflight) == ("reserved", "started", 1)
        body = "Budgeted document body."
        return WebPageResult(
            document=SourceDocument(
                request_url=request.url,
                final_url="https://example.com/articles/final",
                title="Budgeted page",
                text=body,
                text_scope="full",
                observed_at=self._context.now + timedelta(seconds=3),
                published_at=None,
                content_fingerprint=hashlib.sha256(body.encode()).hexdigest(),
                extractor_version="firecrawl/2.11.162",
            ),
            target_status_code=200,
            collector_call_count=1,
            target_request_count=1,
        )


class ExplodingDocumentAdapter:
    def __enter__(self) -> ExplodingDocumentAdapter:
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def fetch_document(self, request: WebPageRequest) -> WebPageResult:
        raise RuntimeError("collector transport failed")


class RateLimitedDocumentAdapter:
    def __init__(self) -> None:
        self.requests: list[WebPageRequest] = []

    def __enter__(self) -> RateLimitedDocumentAdapter:
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def fetch_document(self, request: WebPageRequest) -> WebPageResult:
        self.requests.append(request)
        return WebPageResult(
            stop_reason=SourceStopReason.RATE_LIMITED,
            collector_call_count=1,
        )


class SuccessfulDocumentAdapter:
    def __init__(self, clock: list[datetime]) -> None:
        self._clock = clock
        self.requests: list[WebPageRequest] = []

    def __enter__(self) -> SuccessfulDocumentAdapter:
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def fetch_document(self, request: WebPageRequest) -> WebPageResult:
        self.requests.append(request)
        body = "Worker-persisted document body."
        return WebPageResult(
            document=SourceDocument(
                request_url=request.url,
                final_url="https://example.com/articles/final",
                title="Worker page",
                text=body,
                text_scope="full",
                observed_at=self._clock[0],
                published_at=None,
                content_fingerprint=hashlib.sha256(body.encode()).hexdigest(),
                extractor_version="firecrawl/2.11.162",
            ),
            target_status_code=200,
            collector_call_count=1,
            target_request_count=1,
        )


class StoredWebPageMessage:
    def __init__(self, *, message_id: UUID, job_id: UUID, payload: dict[str, object]) -> None:
        self._message_id = message_id
        self._job_id = job_id
        self._payload = payload

    def topic(self) -> str:
        return "hotkey.jobs.accepted.v2"

    def value(self) -> bytes:
        return json.dumps(
            {
                **self._payload,
                "schema_version": 2,
                "message_id": str(self._message_id),
                "event_type": "job.accepted.v2",
            }
        ).encode()

    def key(self) -> bytes:
        return str(self._job_id).encode()

    def partition(self) -> int:
        return 0

    def offset(self) -> int:
        return 0

    def headers(self) -> list[tuple[str, bytes]]:
        return [("hotkey-message-id", str(self._message_id).encode())]


def _accept_webpage_job(
    context: WebPageContext,
    *,
    clock: list[datetime],
    target_url: str = "https://example.com/articles/one#ignored",
) -> tuple[UUID, Message]:
    with context.engine.begin() as connection:
        connection.execute(text("DELETE FROM jobs WHERE id = :id"), {"id": context.job_id})
    with context.sessions() as session:
        job = WebPageCollectionService(session, clock=lambda: clock[0]).accept_job(
            owner_id=context.owner_id,
            operation_id=context.operation_id,
            target_url=target_url,
        )
    with context.engine.connect() as connection:
        outbox = connection.execute(
            text("SELECT id, payload FROM outbox_messages WHERE aggregate_id = :job_id"),
            {"job_id": job.id},
        ).one()
    return job.id, cast(
        Message,
        StoredWebPageMessage(message_id=outbox.id, job_id=job.id, payload=outbox.payload),
    )


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


def test_firecrawl_call_reserves_and_settles_collector_budget(
    webpage_context: WebPageContext,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = webpage_context.now + timedelta(seconds=2)
    adapter = InspectingDocumentAdapter(webpage_context)

    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id,
            worker_id="web-worker",
        )
        service = WebPageFetchService(
            session,
            lease_seconds=60,
            clock=lambda: clock,
        )
        command = WebPageFetchInput(
            operation_id=webpage_context.operation_id,
            connection_id=webpage_context.connection_id,
            connection_version=1,
            target_url="https://example.com/Articles/One?q=1#ignored",
        )
        fetched = service.fetch_document(
            owner_id=webpage_context.owner_id,
            lease=lease,
            command=command,
            adapter_factory=lambda allowed_hosts: adapter,
        )
        with pytest.raises(UsageConflictError, match="already settled"):
            service.fetch_document(
                owner_id=webpage_context.owner_id,
                lease=fetched.lease,
                command=command,
                adapter_factory=lambda allowed_hosts: adapter,
            )

    assert fetched.result.document is not None
    assert fetched.result.document.request_url == "https://example.com/Articles/One?q=1"
    assert fetched.lease.epoch == lease.epoch
    assert adapter.requests == [WebPageRequest(url="https://example.com/Articles/One?q=1")]
    with webpage_context.engine.connect() as connection:
        settled = connection.execute(
            text(
                "SELECT r.status, r.actual_units, r.released_units, a.outcome, "
                "w.used_units, w.reserved_units, j.requests_sent "
                "FROM resource_budget_reservations r "
                "JOIN resource_usage_attempts a ON a.attempt_id = r.reservation_id "
                "JOIN resource_budget_windows w ON w.id = r.budget_window_id "
                "JOIN jobs j ON j.id = :job_id"
            ),
            {"job_id": webpage_context.job_id},
        ).one()
    assert tuple(settled) == ("settled", 1, 0, "succeeded", 1, 0, 1)


def test_failed_firecrawl_call_is_charged_and_recorded(
    webpage_context: WebPageContext,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = webpage_context.now + timedelta(seconds=2)

    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id,
            worker_id="web-worker",
        )
        with pytest.raises(RuntimeError, match="collector transport failed"):
            WebPageFetchService(
                session,
                lease_seconds=60,
                clock=lambda: clock,
            ).fetch_document(
                owner_id=webpage_context.owner_id,
                lease=lease,
                command=WebPageFetchInput(
                    operation_id=webpage_context.operation_id,
                    connection_id=webpage_context.connection_id,
                    connection_version=1,
                    target_url="https://example.com/articles/one",
                ),
                adapter_factory=lambda allowed_hosts: ExplodingDocumentAdapter(),
            )

    with webpage_context.engine.connect() as connection:
        settled = connection.execute(
            text(
                "SELECT r.status, r.actual_units, a.outcome, w.used_units, "
                "w.reserved_units, j.requests_sent "
                "FROM resource_budget_reservations r "
                "JOIN resource_usage_attempts a ON a.attempt_id = r.reservation_id "
                "JOIN resource_budget_windows w ON w.id = r.budget_window_id "
                "JOIN jobs j ON j.id = :job_id"
            ),
            {"job_id": webpage_context.job_id},
        ).one()
    assert tuple(settled) == ("settled", 1, "failed", 1, 0, 1)


def test_exhausted_collector_budget_stops_before_request(
    webpage_context: WebPageContext,
) -> None:
    _enable_firecrawl_budget(webpage_context, limit_units=1)
    clock = webpage_context.now + timedelta(seconds=2)
    with webpage_context.sessions() as session:
        budget = ResourceBudgetService(session, clock=lambda: clock)
        reservation_id = uuid4()
        budget.reserve_budget(
            owner_id=webpage_context.owner_id,
            command=BudgetReservationInput(
                reservation_id=reservation_id,
                operation_id=uuid4(),
                metric=BudgetMetric.COLLECTOR_CALL,
                requested_units=1,
                context=BudgetContext(source_ref="web"),
            ),
        )
        budget.settle_budget_reservation(
            owner_id=webpage_context.owner_id,
            reservation_id=reservation_id,
            actual_units=1,
        )
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id,
            worker_id="web-worker",
        )
        called = False

        def adapter_factory(allowed_hosts: frozenset[str]) -> ExplodingDocumentAdapter:
            nonlocal called
            called = True
            return ExplodingDocumentAdapter()

        with pytest.raises(WebPageBudgetDelayedError) as error:
            WebPageFetchService(
                session,
                lease_seconds=60,
                clock=lambda: clock,
            ).fetch_document(
                owner_id=webpage_context.owner_id,
                lease=lease,
                command=WebPageFetchInput(
                    operation_id=webpage_context.operation_id,
                    connection_id=webpage_context.connection_id,
                    connection_version=1,
                    target_url="https://example.com/articles/one",
                ),
                adapter_factory=adapter_factory,
            )

    assert not called
    assert error.value.decision.remaining_units == 0
    assert error.value.decision.retry_at == webpage_context.now + timedelta(seconds=60)
    with webpage_context.engine.connect() as connection:
        counts = connection.execute(
            text(
                "SELECT (SELECT count(*) FROM resource_budget_reservations), "
                "(SELECT count(*) FROM resource_usage_attempts), "
                "(SELECT requests_sent FROM jobs WHERE id = :job_id)"
            ),
            {"job_id": webpage_context.job_id},
        ).one()
    assert tuple(counts) == (1, 0, 0)


def test_connection_version_conflict_stops_before_budget_or_request(
    webpage_context: WebPageContext,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = webpage_context.now + timedelta(seconds=2)
    called = False

    def adapter_factory(allowed_hosts: frozenset[str]) -> ExplodingDocumentAdapter:
        nonlocal called
        called = True
        return ExplodingDocumentAdapter()

    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id,
            worker_id="web-worker",
        )
        with pytest.raises(ApplicationError, match="connection_version_conflict"):
            WebPageFetchService(
                session,
                lease_seconds=60,
                clock=lambda: clock,
            ).fetch_document(
                owner_id=webpage_context.owner_id,
                lease=lease,
                command=WebPageFetchInput(
                    operation_id=webpage_context.operation_id,
                    connection_id=webpage_context.connection_id,
                    connection_version=2,
                    target_url="https://example.com/articles/one",
                ),
                adapter_factory=adapter_factory,
            )

    assert not called
    with webpage_context.engine.connect() as connection:
        counts = connection.execute(
            text(
                "SELECT (SELECT count(*) FROM resource_budget_reservations), "
                "(SELECT count(*) FROM resource_usage_attempts), "
                "(SELECT count(*) FROM resource_budget_windows), "
                "(SELECT requests_sent FROM jobs WHERE id = :job_id)"
            ),
            {"job_id": webpage_context.job_id},
        ).one()
    assert tuple(counts) == (0, 0, 0, 0)


def test_abandoned_collector_attempt_is_conservatively_charged_before_retry(
    webpage_context: WebPageContext,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _enable_firecrawl_budget(webpage_context, limit_units=2)
    clock = [webpage_context.now + timedelta(seconds=2)]
    adapter = SuccessfulDocumentAdapter(clock)
    command = WebPageFetchInput(
        operation_id=webpage_context.operation_id,
        connection_id=webpage_context.connection_id,
        connection_version=1,
        target_url="https://example.com/articles/one",
    )
    original_settle = WebPageFetchService._settle

    def interrupt_before_settlement(*args: object, **kwargs: object) -> None:
        raise RuntimeError("injected crash before settlement")

    with webpage_context.sessions() as session:
        old_lease = JobExecutionService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).acquire(job_id=webpage_context.job_id, worker_id="old-worker")
        monkeypatch.setattr(WebPageFetchService, "_settle", interrupt_before_settlement)
        with pytest.raises(RuntimeError, match="crash before settlement"):
            WebPageFetchService(
                session,
                lease_seconds=60,
                clock=lambda: clock[0],
            ).fetch_document(
                owner_id=webpage_context.owner_id,
                lease=old_lease,
                command=command,
                adapter_factory=lambda allowed_hosts: adapter,
            )

    clock[0] += timedelta(seconds=61)
    monkeypatch.setattr(WebPageFetchService, "_settle", original_settle)
    with webpage_context.sessions() as session:
        recovered_lease = JobExecutionService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).acquire(job_id=webpage_context.job_id, worker_id="new-worker")
        fetched = WebPageFetchService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).fetch_document(
            owner_id=webpage_context.owner_id,
            lease=recovered_lease,
            command=command,
            adapter_factory=lambda allowed_hosts: adapter,
        )

    assert fetched.result.document is not None
    assert len(adapter.requests) == 2
    with webpage_context.engine.connect() as connection:
        attempts = connection.execute(
            text(
                "SELECT a.outcome, r.status, r.actual_units "
                "FROM resource_usage_attempts a "
                "JOIN resource_budget_reservations r ON r.reservation_id = a.attempt_id "
                "ORDER BY a.started_at, a.attempt_id"
            )
        ).all()
        window = connection.execute(
            text(
                "SELECT coalesce(sum(used_units), 0), coalesce(sum(reserved_units), 0) "
                "FROM resource_budget_windows"
            )
        ).one()
    assert attempts == [("failed", "settled", 1), ("succeeded", "settled", 1)]
    assert tuple(window) == (2, 0)


def test_supervised_terminal_write_recovers_usage_and_inbox_atomically(
    webpage_context: WebPageContext,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = [webpage_context.now + timedelta(seconds=2)]
    job_id, kafka_message = _accept_webpage_job(webpage_context, clock=clock)
    body, reference = decode_job_message(kafka_message)
    with webpage_context.sessions() as session:
        lease = JobExecutionService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).acquire(job_id=job_id, worker_id="worker-supervised")

    def interrupt_settlement(*args: object, **kwargs: object) -> None:
        raise RuntimeError("injected interruption after request start")

    monkeypatch.setattr(WebPageFetchService, "_settle", interrupt_settlement)
    with (
        webpage_context.sessions() as session,
        pytest.raises(RuntimeError, match="injected interruption"),
    ):
        WebPageFetchService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).fetch_document(
            owner_id=webpage_context.owner_id,
            lease=lease,
            command=WebPageFetchInput(
                operation_id=body.operation_id,
                connection_id=webpage_context.connection_id,
                connection_version=1,
                target_url="https://example.com/articles/one",
            ),
            adapter_factory=lambda _hosts: ExplodingDocumentAdapter(),
        )

    failure = JobExecutionFailure(
        error_code="job_execution_timeout",
        category=JobFailureCategory.TRANSIENT,
        occurred_at=clock[0],
        next_action="确认目标服务状态后手动重试",
        manual_retry_allowed=True,
    )
    report = ChildJobResult(
        lease=lease,
        failure=ChildJobFailure.from_failure(failure),
    )
    original_record_failure = JobExecutionService.record_failure_in_transaction

    def fail_after_job_write(
        service: JobExecutionService,
        *args: object,
        **kwargs: object,
    ) -> None:
        original_record_failure(service, *args, **kwargs)  # type: ignore[arg-type]
        raise RuntimeError("injected transaction rollback")

    monkeypatch.setattr(
        JobExecutionService,
        "record_failure_in_transaction",
        fail_after_job_write,
    )
    with pytest.raises(RuntimeError, match="injected transaction rollback"):
        _finalize_supervised_result(
            webpage_context.sessions,
            message=body,
            reference=reference,
            report=report,
            lease_seconds=60,
            clock=lambda: clock[0],
        )

    with webpage_context.engine.connect() as connection:
        after_rollback = connection.execute(
            text(
                "SELECT a.outcome, r.status, j.status, "
                "(SELECT count(*) FROM processed_messages p WHERE p.job_id = j.id) "
                "FROM jobs j "
                "JOIN resource_usage_attempts a ON a.operation_id = j.operation_id "
                "JOIN resource_budget_reservations r ON r.reservation_id = a.attempt_id "
                "WHERE j.id = :job_id"
            ),
            {"job_id": job_id},
        ).one()
    assert tuple(after_rollback) == ("started", "reserved", "running", 0)

    monkeypatch.setattr(
        JobExecutionService,
        "record_failure_in_transaction",
        original_record_failure,
    )
    _finalize_supervised_result(
        webpage_context.sessions,
        message=body,
        reference=reference,
        report=report,
        lease_seconds=60,
        clock=lambda: clock[0],
    )

    with webpage_context.engine.connect() as connection:
        committed = connection.execute(
            text(
                "SELECT a.outcome, r.status, r.actual_units, j.status, "
                "(SELECT count(*) FROM processed_messages p WHERE p.job_id = j.id) "
                "FROM jobs j "
                "JOIN resource_usage_attempts a ON a.operation_id = j.operation_id "
                "JOIN resource_budget_reservations r ON r.reservation_id = a.attempt_id "
                "WHERE j.id = :job_id"
            ),
            {"job_id": job_id},
        ).one()
    assert tuple(committed) == ("failed", "settled", 1, "failed", 1)


def test_worker_executes_job_in_a_spawned_process(
    webpage_context: WebPageContext,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = [datetime.now(UTC)]
    job_id, message = _accept_webpage_job(webpage_context, clock=clock)
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    assert database_url is not None
    monkeypatch.setenv("HOTKEY_DATABASE_URL", database_url)
    monkeypatch.setenv("HOTKEY_FIRECRAWL_ENABLED", "false")
    handler = create_job_message_handler(
        webpage_context.sessions,
        {"webpage.collect": lambda _context: None},
        worker_id="worker-spawn-integration",
        lease_seconds=60,
        supervisor=JobProcessSupervisor(
            startup_timeout_seconds=5,
            execution_timeout_seconds=10,
            terminate_grace_seconds=1,
            poll_interval_seconds=0.05,
        ),
        stopping=Event(),
    )

    handler(message)

    with webpage_context.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT status, last_error_code, "
                "(SELECT outcome FROM job_attempts WHERE job_id = jobs.id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = jobs.id) "
                "FROM jobs WHERE id = :job_id"
            ),
            {"job_id": job_id},
        ).one()
    assert tuple(row) == ("failed", "collector_unavailable", "failed", 1)


def test_worker_executes_webpage_job_and_replay_has_no_duplicate_effects(
    webpage_context: WebPageContext,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = [webpage_context.now + timedelta(seconds=2)]
    job_id, message = _accept_webpage_job(webpage_context, clock=clock)
    clock[0] += timedelta(seconds=1)
    adapter = SuccessfulDocumentAdapter(clock)
    executor = WebPageCollectionExecutor(
        webpage_context.sessions,
        lease_seconds=60,
        adapter_factory=lambda allowed_hosts: adapter,
        clock=lambda: clock[0],
    )

    def collect(context: JobExecutionContext) -> None:
        context.lease = executor.execute(context.message, context.lease)

    handler = create_job_message_handler(
        webpage_context.sessions,
        {"webpage.collect": collect},
        worker_id="web-worker",
        lease_seconds=60,
        clock=lambda: clock[0],
    )
    handler(message)
    handler(message)

    with webpage_context.sessions() as session:
        status = JobService(session, clock=lambda: clock[0]).get_status(
            owner_id=webpage_context.owner_id,
            job_id=job_id,
        )
    assert status.status == "succeeded"
    assert status.result_content_id is not None
    assert status.progress.requests_sent == 1
    assert status.progress.items_saved == 1
    assert len(adapter.requests) == 1
    assert _content_side_effect_counts(webpage_context) == (1, 1, 1, 1, 1, 1, 1)
    with webpage_context.engine.connect() as connection:
        counts = connection.execute(
            text(
                "SELECT (SELECT count(*) FROM resource_usage_attempts), "
                "(SELECT count(*) FROM processed_messages), "
                "(SELECT count(*) FROM job_attempts WHERE job_id = :job_id)"
            ),
            {"job_id": job_id},
        ).one()
        evidence = connection.execute(
            text(
                "SELECT usage.attempt_id, usage.operation_id, evidence.operation_id, "
                "evidence.outcome, evidence.stop_reason "
                "FROM jobs job "
                "JOIN resource_usage_attempts usage "
                "ON usage.owner_id = job.owner_id AND usage.operation_id = job.operation_id "
                "JOIN source_capability_evidence evidence "
                "ON evidence.owner_id = usage.owner_id "
                "AND evidence.operation_id = usage.attempt_id "
                "WHERE job.id = :job_id AND usage.usage_kind = 'collector_call'"
            ),
            {"job_id": job_id},
        ).one()
    assert tuple(counts) == (1, 1, 1)
    expected_attempt_id = resource_attempt_id(
        operation_id=webpage_context.operation_id,
        component_key="collector.firecrawl",
        stage="page_content.fetch",
        sequence=1,
    )
    assert tuple(evidence) == (
        expected_attempt_id,
        webpage_context.operation_id,
        expected_attempt_id,
        "succeeded",
        None,
    )


def test_real_kafka_delivers_webpage_job_to_persisted_result(
    webpage_context: WebPageContext,
) -> None:
    bootstrap_servers = os.getenv("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS")
    if bootstrap_servers is None:
        pytest.skip("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS is required for Kafka integration")
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    _enable_firecrawl_budget(webpage_context)
    clock = [webpage_context.now + timedelta(seconds=2)]
    live_firecrawl = os.getenv("HOTKEY_TEST_LIVE_FIRECRAWL") == "1"
    job_id, _ = _accept_webpage_job(
        webpage_context,
        clock=clock,
        target_url=(
            "https://example.com/" if live_firecrawl else "https://example.com/articles/one"
        ),
    )
    topic = f"hotkey.tests.webpage.{uuid4().hex}"
    group_id = f"hotkey-tests-{uuid4().hex}"
    admin = AdminClient({"bootstrap.servers": bootstrap_servers})
    admin.create_topics([NewTopic(topic, num_partitions=1, replication_factor=1)])[topic].result(10)
    with webpage_context.engine.begin() as connection:
        connection.execute(
            text("UPDATE outbox_messages SET topic = :topic WHERE aggregate_id = :job_id"),
            {"topic": topic, "job_id": job_id},
        )

    consumer = Consumer(
        {
            "bootstrap.servers": bootstrap_servers,
            "group.id": group_id,
            "enable.auto.commit": False,
            "enable.auto.offset.store": False,
            "auto.offset.reset": "earliest",
            "session.timeout.ms": 6_000,
            "max.poll.interval.ms": 120_000,
        }
    )
    try:
        consumer.subscribe([topic])
        _wait_for_assignment(consumer)
        producer = create_producer(
            Settings(
                database_url=database_url,
                kafka_bootstrap_servers=bootstrap_servers,
                kafka_delivery_timeout_seconds=10,
            )
        )
        with webpage_context.sessions() as session:
            assert (
                OutboxService(session).publish_pending(
                    lambda envelope: publish_outbox(producer, envelope, timeout_seconds=10),
                    published_at=clock[0],
                )
                == 1
            )
        kafka_message = _poll_message(consumer)
        clock[0] += timedelta(seconds=1)
        adapter = SuccessfulDocumentAdapter(clock)

        def adapter_factory(allowed_hosts: frozenset[str]):
            if live_firecrawl:
                return FirecrawlAdapter(
                    base_url=os.getenv(
                        "HOTKEY_FIRECRAWL_BASE_URL",
                        "http://127.0.0.1:3002",
                    ),
                    enabled=True,
                    allowed_hosts=allowed_hosts,
                    clock=lambda: clock[0],
                )
            return adapter

        executor = WebPageCollectionExecutor(
            webpage_context.sessions,
            lease_seconds=60,
            adapter_factory=adapter_factory,
            clock=lambda: clock[0],
        )

        def collect(context: JobExecutionContext) -> None:
            context.lease = executor.execute(context.message, context.lease)

        create_job_message_handler(
            webpage_context.sessions,
            {"webpage.collect": collect},
            worker_id="kafka-web-worker",
            lease_seconds=60,
            clock=lambda: clock[0],
        )(kafka_message)
        consumer.commit(message=kafka_message, asynchronous=False)

        with webpage_context.sessions() as session:
            status = JobService(session, clock=lambda: clock[0]).get_status(
                owner_id=webpage_context.owner_id,
                job_id=job_id,
            )
        assert status.status == "succeeded", status.model_dump(mode="json")
        assert status.result_content_id is not None
        if not live_firecrawl:
            assert len(adapter.requests) == 1
        assert _content_side_effect_counts(webpage_context) == (1, 1, 1, 1, 1, 1, 1)
    finally:
        consumer.close()
        with suppress(Exception):
            admin.delete_topics([topic], operation_timeout=10)[topic].result(10)


def test_worker_recovers_committed_checkpoint_without_second_source_call(
    webpage_context: WebPageContext,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = [webpage_context.now + timedelta(seconds=2)]
    job_id, message = _accept_webpage_job(webpage_context, clock=clock)
    clock[0] += timedelta(seconds=1)
    adapter = SuccessfulDocumentAdapter(clock)
    first_executor = WebPageCollectionExecutor(
        webpage_context.sessions,
        lease_seconds=60,
        adapter_factory=lambda allowed_hosts: adapter,
        clock=lambda: clock[0],
    )
    body, _ = decode_job_message(message)
    with webpage_context.sessions() as session:
        old_lease = JobExecutionService(
            session,
            lease_seconds=60,
            clock=lambda: clock[0],
        ).acquire(job_id=job_id, worker_id="interrupted-worker")
    committed_lease = first_executor.execute(body, old_lease)
    assert committed_lease.checkpoint_sequence == 1

    clock[0] += timedelta(seconds=61)

    def fail_if_called(allowed_hosts: frozenset[str]) -> ExplodingDocumentAdapter:
        raise AssertionError("recovery must not call Firecrawl again")

    recovery_executor = WebPageCollectionExecutor(
        webpage_context.sessions,
        lease_seconds=60,
        adapter_factory=fail_if_called,
        clock=lambda: clock[0],
    )

    def recover(context: JobExecutionContext) -> None:
        context.lease = recovery_executor.execute(context.message, context.lease)

    create_job_message_handler(
        webpage_context.sessions,
        {"webpage.collect": recover},
        worker_id="recovery-worker",
        lease_seconds=60,
        clock=lambda: clock[0],
    )(message)

    with webpage_context.sessions() as session:
        status = JobService(session, clock=lambda: clock[0]).get_status(
            owner_id=webpage_context.owner_id,
            job_id=job_id,
        )
    assert status.status == "succeeded"
    assert status.result_content_id is not None
    assert len(adapter.requests) == 1
    assert _content_side_effect_counts(webpage_context) == (1, 1, 1, 1, 1, 1, 1)
    with webpage_context.engine.connect() as connection:
        attempts = connection.execute(
            text(
                "SELECT lease_epoch, outcome FROM job_attempts "
                "WHERE job_id = :job_id ORDER BY lease_epoch"
            ),
            {"job_id": job_id},
        ).all()
        processed = connection.execute(text("SELECT count(*) FROM processed_messages")).scalar_one()
    assert attempts == [(1, "expired"), (2, "succeeded")]
    assert processed == 1


def test_worker_persists_connection_replacement_failure_without_source_call(
    webpage_context: WebPageContext,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = [webpage_context.now + timedelta(seconds=2)]
    job_id, message = _accept_webpage_job(webpage_context, clock=clock)
    clock[0] += timedelta(seconds=1)
    with webpage_context.engine.begin() as connection:
        connection.execute(text("SET CONSTRAINTS ALL DEFERRED"))
        connection.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, auth_kind, secret_ref, configuration, "
                "created_by, created_at) VALUES "
                "(:id, 2, :owner_id, 'none', NULL, "
                "CAST(:configuration AS jsonb), :owner_id, :now)"
            ),
            {
                "id": webpage_context.connection_id,
                "owner_id": webpage_context.owner_id,
                "configuration": json.dumps({"allowed_hosts": ["example.com"]}),
                "now": clock[0],
            },
        )
        connection.execute(
            text("UPDATE source_connections SET current_version = 2, updated_at = :now"),
            {"now": clock[0]},
        )

    called = False

    def adapter_factory(allowed_hosts: frozenset[str]) -> ExplodingDocumentAdapter:
        nonlocal called
        called = True
        return ExplodingDocumentAdapter()

    executor = WebPageCollectionExecutor(
        webpage_context.sessions,
        lease_seconds=60,
        adapter_factory=adapter_factory,
        clock=lambda: clock[0],
    )

    def collect(context: JobExecutionContext) -> None:
        context.lease = executor.execute(context.message, context.lease)

    create_job_message_handler(
        webpage_context.sessions,
        {"webpage.collect": collect},
        worker_id="web-worker",
        lease_seconds=60,
        clock=lambda: clock[0],
    )(message)

    with webpage_context.sessions() as session:
        status = JobService(session, clock=lambda: clock[0]).get_status(
            owner_id=webpage_context.owner_id,
            job_id=job_id,
        )
    assert not called
    assert status.status == "failed"
    assert status.failure is not None
    assert status.failure.error_code == "source_connection_changed"
    assert status.failure.category == "configuration_unavailable"
    assert status.result_content_id is None
    with webpage_context.engine.connect() as connection:
        counts = connection.execute(
            text(
                "SELECT (SELECT count(*) FROM content_records), "
                "(SELECT count(*) FROM resource_usage_attempts), "
                "(SELECT count(*) FROM resource_budget_reservations), "
                "(SELECT count(*) FROM processed_messages)"
            )
        ).one()
    assert tuple(counts) == (0, 0, 0, 1)


def test_worker_persists_rate_limit_evidence_and_schedules_bounded_retry(
    webpage_context: WebPageContext,
) -> None:
    _enable_firecrawl_budget(webpage_context)
    clock = [webpage_context.now + timedelta(seconds=2)]
    job_id, message = _accept_webpage_job(webpage_context, clock=clock)
    clock[0] += timedelta(seconds=1)
    adapter = RateLimitedDocumentAdapter()
    executor = WebPageCollectionExecutor(
        webpage_context.sessions,
        lease_seconds=60,
        adapter_factory=lambda allowed_hosts: adapter,
        clock=lambda: clock[0],
    )

    def collect(context: JobExecutionContext) -> None:
        context.lease = executor.execute(context.message, context.lease)

    create_job_message_handler(
        webpage_context.sessions,
        {"webpage.collect": collect},
        worker_id="web-worker",
        lease_seconds=60,
        clock=lambda: clock[0],
    )(message)

    with webpage_context.sessions() as session:
        status = JobService(session, clock=lambda: clock[0]).get_status(
            owner_id=webpage_context.owner_id,
            job_id=job_id,
        )
    assert len(adapter.requests) == 1
    assert status.status == "queued"
    assert status.failure is not None
    assert status.failure.error_code == "source_rate_limited"
    assert status.failure.category == "rate_limited"
    assert status.retry_count == 1
    assert status.next_run_at == clock[0] + timedelta(seconds=30)
    assert status.result_content_id is None
    with webpage_context.engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT (SELECT count(*) FROM content_records), "
                "(SELECT count(*) FROM source_capability_evidence "
                " WHERE outcome = 'failed' AND stop_reason = 'rate_limited'), "
                "(SELECT count(*) FROM resource_usage_attempts WHERE outcome = 'failed'), "
                "(SELECT count(*) FROM outbox_messages WHERE aggregate_id = :job_id), "
                "(SELECT count(*) FROM processed_messages WHERE job_id = :job_id)"
            ),
            {"job_id": job_id},
        ).one()
        evidence = connection.execute(
            text(
                "SELECT usage.attempt_id, usage.operation_id, evidence.operation_id, "
                "evidence.outcome, evidence.stop_reason "
                "FROM jobs job "
                "JOIN resource_usage_attempts usage "
                "ON usage.owner_id = job.owner_id AND usage.operation_id = job.operation_id "
                "JOIN source_capability_evidence evidence "
                "ON evidence.owner_id = usage.owner_id "
                "AND evidence.operation_id = usage.attempt_id "
                "WHERE job.id = :job_id AND usage.usage_kind = 'collector_call'"
            ),
            {"job_id": job_id},
        ).one()
    assert tuple(row) == (0, 1, 1, 2, 1)
    expected_attempt_id = resource_attempt_id(
        operation_id=webpage_context.operation_id,
        component_key="collector.firecrawl",
        stage="page_content.fetch",
        sequence=1,
    )
    assert tuple(evidence) == (
        expected_attempt_id,
        webpage_context.operation_id,
        expected_attempt_id,
        "failed",
        "rate_limited",
    )


def test_document_page_is_persisted_with_checkpoint_in_one_transaction(
    webpage_context: WebPageContext,
) -> None:
    clock = webpage_context.now + timedelta(seconds=2)
    command = _command(webpage_context)
    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id, worker_id="web-worker"
        )
        service = WebPageCommitService(session, lease_seconds=60, clock=lambda: clock)
        first, renewed = service.commit_document_page(
            owner_id=webpage_context.owner_id,
            lease=lease,
            command=command,
            sequence=1,
        )
        replayed, replayed_lease = service.commit_document_page(
            owner_id=webpage_context.owner_id,
            lease=renewed,
            command=command,
            sequence=1,
        )

    normalized_url = "https://example.com/Articles/One?q=1"
    assert first.id == replayed.id
    assert first.object_type == "webpage"
    assert first.native_scope == "example.com"
    assert first.external_id == hashlib.sha256(normalized_url.encode()).hexdigest()
    assert first.latest_observation.id == replayed.latest_observation.id
    assert first.latest_observation.canonical_url == normalized_url
    assert first.latest_observation.final_url == "https://example.com/articles/final?q=1"
    assert first.latest_observation.metrics.model_dump() == {
        "like_count": None,
        "comment_count": None,
        "repost_count": None,
        "view_count": None,
        "play_count": None,
        "danmaku_count": None,
    }
    assert first.latest_observation.content_version is not None
    assert first.latest_observation.content_version.text_origin == "machine_extracted"
    assert first.latest_observation.content_version.text_origin_ref == "firecrawl/2.11.162"
    assert _content_side_effect_counts(webpage_context) == (1, 1, 1, 1, 1, 1, 1)
    assert replayed_lease.checkpoint_sequence == 1
    assert replayed_lease.checkpoint == {
        "content_id": str(first.id),
        "observation_id": str(first.latest_observation.id),
    }
    with webpage_context.engine.connect() as connection:
        job_row = connection.execute(
            text(
                "SELECT checkpoint_sequence, checkpoint, progress_stage, items_saved "
                "FROM jobs WHERE id = :id"
            ),
            {"id": webpage_context.job_id},
        ).one()
    assert tuple(job_row) == (1, replayed_lease.checkpoint, "save", 1)


def test_stale_epoch_cannot_write_document_or_checkpoint(
    webpage_context: WebPageContext,
) -> None:
    clock = [webpage_context.now + timedelta(seconds=2)]
    with webpage_context.sessions() as session:
        old = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0]).acquire(
            job_id=webpage_context.job_id, worker_id="old-worker"
        )
    clock[0] += timedelta(seconds=61)
    with webpage_context.sessions() as session:
        JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0]).acquire(
            job_id=webpage_context.job_id,
            worker_id="new-worker",
        )
    with webpage_context.sessions() as session, pytest.raises(StaleExecutionLeaseError):
        WebPageCommitService(
            session, lease_seconds=60, clock=lambda: clock[0]
        ).commit_document_page(
            owner_id=webpage_context.owner_id,
            lease=old,
            command=_command(webpage_context),
            sequence=1,
        )

    assert _content_side_effect_counts(webpage_context) == (0, 0, 0, 0, 0, 0, 0)


def test_connection_replacement_cannot_be_attributed_to_old_version(
    webpage_context: WebPageContext,
) -> None:
    clock = webpage_context.now + timedelta(seconds=2)
    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id, worker_id="web-worker"
        )
    with webpage_context.engine.begin() as connection:
        connection.execute(text("SET CONSTRAINTS ALL DEFERRED"))
        connection.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, auth_kind, secret_ref, configuration, "
                "created_by, created_at) VALUES "
                "(:id, 2, :owner_id, 'none', NULL, "
                '\'{"allowed_hosts":["example.com"]}\'::jsonb, :owner_id, :now)'
            ),
            {
                "id": webpage_context.connection_id,
                "owner_id": webpage_context.owner_id,
                "now": clock,
            },
        )
        connection.execute(
            text("UPDATE source_connections SET current_version = 2, updated_at = :now"),
            {"now": clock},
        )

    with (
        webpage_context.sessions() as session,
        pytest.raises(ApplicationError, match="connection_version_conflict"),
    ):
        WebPageCommitService(session, lease_seconds=60, clock=lambda: clock).commit_document_page(
            owner_id=webpage_context.owner_id,
            lease=lease,
            command=_command(webpage_context),
            sequence=1,
        )

    assert _content_side_effect_counts(webpage_context) == (0, 0, 0, 0, 0, 0, 0)
    with webpage_context.engine.connect() as connection:
        assert (
            connection.execute(
                text("SELECT checkpoint_sequence FROM jobs WHERE id = :id"),
                {"id": webpage_context.job_id},
            ).scalar_one()
            == 0
        )


def test_cancelled_job_cannot_write_document_or_checkpoint(
    webpage_context: WebPageContext,
) -> None:
    clock = webpage_context.now + timedelta(seconds=2)
    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id, worker_id="web-worker"
        )
    with webpage_context.engine.begin() as connection:
        connection.execute(
            text(
                "UPDATE jobs SET cancel_requested_at = :now, cancel_deadline_at = :deadline "
                "WHERE id = :id"
            ),
            {
                "id": webpage_context.job_id,
                "now": clock,
                "deadline": clock + timedelta(seconds=10),
            },
        )

    with webpage_context.sessions() as session, pytest.raises(JobLeaseUnavailableError):
        WebPageCommitService(session, lease_seconds=60, clock=lambda: clock).commit_document_page(
            owner_id=webpage_context.owner_id,
            lease=lease,
            command=_command(webpage_context),
            sequence=1,
        )

    assert _content_side_effect_counts(webpage_context) == (0, 0, 0, 0, 0, 0, 0)


def test_final_redirect_outside_connection_scope_is_rejected_before_write(
    webpage_context: WebPageContext,
) -> None:
    clock = webpage_context.now + timedelta(seconds=2)
    command = _command(webpage_context)
    fields = {**command.admission.fields, "final_url": "https://other.example/final"}
    rejected = command.model_copy(
        update={"admission": command.admission.model_copy(update={"fields": fields})}
    )
    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id, worker_id="web-worker"
        )
        with pytest.raises(ApplicationError, match="source_target_not_allowed"):
            WebPageCommitService(
                session, lease_seconds=60, clock=lambda: clock
            ).commit_document_page(
                owner_id=webpage_context.owner_id,
                lease=lease,
                command=rejected,
                sequence=1,
            )

    assert _content_side_effect_counts(webpage_context) == (0, 0, 0, 0, 0, 0, 0)


def test_revoked_policy_cannot_write_document_or_checkpoint(
    webpage_context: WebPageContext,
) -> None:
    clock = webpage_context.now + timedelta(seconds=2)
    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id, worker_id="web-worker"
        )
    with webpage_context.engine.begin() as connection:
        connection.execute(
            text(
                "UPDATE source_access_policies SET enabled = false, updated_at = :now "
                "WHERE id = :id"
            ),
            {"id": webpage_context.policy_id, "now": clock},
        )

    with webpage_context.sessions() as session, pytest.raises(ResourceUnavailableError):
        WebPageCommitService(session, lease_seconds=60, clock=lambda: clock).commit_document_page(
            owner_id=webpage_context.owner_id,
            lease=lease,
            command=_command(webpage_context),
            sequence=1,
        )

    assert _content_side_effect_counts(webpage_context) == (0, 0, 0, 0, 0, 0, 0)


def test_checkpoint_failure_rolls_back_document_and_evidence(
    webpage_context: WebPageContext,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    clock = webpage_context.now + timedelta(seconds=2)
    with webpage_context.sessions() as session:
        lease = JobExecutionService(session, lease_seconds=60, clock=lambda: clock).acquire(
            job_id=webpage_context.job_id, worker_id="web-worker"
        )

        def fail_checkpoint(*args: object, **kwargs: object) -> None:
            raise RuntimeError("checkpoint storage failed")

        monkeypatch.setattr(
            JobExecutionService,
            "save_checkpoint_in_transaction",
            fail_checkpoint,
        )
        with pytest.raises(RuntimeError, match="checkpoint storage failed"):
            WebPageCommitService(
                session, lease_seconds=60, clock=lambda: clock
            ).commit_document_page(
                owner_id=webpage_context.owner_id,
                lease=lease,
                command=_command(webpage_context),
                sequence=1,
            )

    assert _content_side_effect_counts(webpage_context) == (0, 0, 0, 0, 0, 0, 0)
    with webpage_context.engine.connect() as connection:
        assert (
            connection.execute(
                text("SELECT checkpoint_sequence FROM jobs WHERE id = :id"),
                {"id": webpage_context.job_id},
            ).scalar_one()
            == 0
        )
