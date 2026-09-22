from __future__ import annotations

import hashlib
import json
import os
from collections.abc import Iterator
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, text
from sqlalchemy.orm import Session, sessionmaker

from connections.schemas import SourceEntryPoint
from content.collection import (
    WebPageBudgetDelayedError,
    WebPageCommitService,
    WebPageFetchInput,
    WebPageFetchService,
)
from content.schemas import PersistContentDocumentInput
from core.errors import ApplicationError
from evidence.schemas import AdmittedSourcePayload, DataClass
from evidence.services import ResourceUnavailableError
from jobs.execution import (
    JobExecutionService,
    JobLeaseUnavailableError,
    StaleExecutionLeaseError,
)
from jobs.schemas import (
    BudgetContext,
    BudgetMetric,
    BudgetPolicyInput,
    BudgetReservationInput,
    BudgetScopeKind,
    ComponentPolicyInput,
    CostClass,
)
from jobs.services import ResourceBudgetService, UsageConflictError
from sources.contracts import (
    SourceCapability,
    SourceDocument,
    WebPageRequest,
    WebPageResult,
)

_TABLES = (
    "content_version_relations, content_visibility_observations, content_observations, "
    "content_versions, content_discoveries, content_records, source_capability_evidence, "
    "source_connection_versions, source_connections, provenance_manifest_inputs, "
    "provenance_manifests, evidence_cleanup_targets, evidence_deletions, "
    "evidence_resources, evidence_retention_policies, source_access_policies, "
    "resource_budget_reservations, resource_budget_windows, resource_budget_policies, "
    "resource_usage_attempts, resource_component_policies, job_stage_attempts, "
    "processed_messages, job_attempts, outbox_messages, jobs, monitor_topic_versions, "
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
