from __future__ import annotations

import json
import os
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from sqlalchemy import create_engine, text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session, sessionmaker

from analysis.services import AnalysisService
from connections.schemas import SourceExecutionPolicy, SourceQuietWindow
from content.services import ContentService
from jobs.coverage import CollectionDueWindowService
from jobs.schemas import (
    BudgetContext,
    BudgetMetric,
    BudgetPolicyInput,
    BudgetReservationInput,
    BudgetScopeKind,
    CollectionDueWindowInput,
    ComponentPolicyInput,
    CostClass,
    CoverageWindowStatus,
    DueAdmissionState,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
)
from jobs.services import ResourceBudgetService
from sources.contracts import SourceCapability


@pytest.fixture
def coverage_context() -> tuple[sessionmaker[Session], UUID, datetime]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")
    engine = create_engine(database_url)
    sessions = sessionmaker(bind=engine, expire_on_commit=False)
    owner_id = uuid4()
    now = datetime(2026, 9, 27, 0, 0, tzinfo=UTC)
    with engine.begin() as connection:
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:id, :username, 'test-only-hash', 1, :now, :now)"
            ),
            {"id": owner_id, "username": f"coverage-{owner_id.hex}", "now": now},
        )
    try:
        yield sessions, owner_id, now
    finally:
        with engine.begin() as connection:
            connection.execute(
                text("DELETE FROM collection_due_windows WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            )
            connection.execute(
                text("DELETE FROM content_records WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            )
            connection.execute(
                text("DELETE FROM monitor_topic_versions WHERE created_by = :owner_id"),
                {"owner_id": owner_id},
            )
            connection.execute(
                text("DELETE FROM monitor_topics WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            )
            connection.execute(
                text("DELETE FROM identity_users WHERE id = :owner_id"),
                {"owner_id": owner_id},
            )
        engine.dispose()


def _due(
    owner_id: UUID, now: datetime, *, schedule_key: UUID | None = None
) -> CollectionDueWindowInput:
    return CollectionDueWindowInput(
        owner_id=owner_id,
        schedule_key=schedule_key or uuid4(),
        topic_id=None,
        source_key="bilibili",
        capability=SourceCapability.SEARCH,
        due_at=now,
        window_start=now - timedelta(hours=6),
        window_end=now,
        connection_version=2,
        policy_snapshot=SourceExecutionPolicy(
            min_interval_seconds=6 * 3600,
            quiet_windows=(),
            max_queries=1,
            max_items_per_query=5,
            max_requests=26,
            max_seconds=220,
            hard_timeout_seconds=240,
            max_concurrency=1,
            enabled=True,
        ),
    )


def test_same_due_point_is_idempotent_under_concurrent_transactions(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    command = _due(owner_id, now)

    def record() -> UUID:
        with sessions() as session, session.begin():
            return CollectionDueWindowService(session).record_due_in_transaction(command).id

    with ThreadPoolExecutor(max_workers=2) as pool:
        ids = list(pool.map(lambda _: record(), range(2)))
    assert ids[0] == ids[1]
    with sessions() as session:
        rows = CollectionDueWindowService(session).list_due(
            owner_id=owner_id, start=now, end=now + timedelta(seconds=1)
        )
    assert len(rows) == 1
    assert rows[0].admission_state is DueAdmissionState.PENDING
    assert rows[0].job_id is None


def test_recovery_records_each_missed_due_without_a_job_and_keeps_old_policy(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    command = _due(owner_id, now - timedelta(hours=18))
    with sessions() as session, session.begin():
        service = CollectionDueWindowService(session)
        first = service.record_due_in_transaction(command)
        recovered = service.record_elapsed_in_transaction(
            first_due=command,
            interval_seconds=6 * 3600,
            through_at=now,
            snapshot_at=lambda _: (command.connection_version, command.policy_snapshot),
        )
    assert [item.due_at for item in recovered] == [
        now - timedelta(hours=18),
        now - timedelta(hours=12),
        now - timedelta(hours=6),
        now,
    ]
    assert recovered[0].id == first.id
    assert [item.admission_state for item in recovered] == [
        DueAdmissionState.MISSED,
        DueAdmissionState.MISSED,
        DueAdmissionState.MISSED,
        DueAdmissionState.PENDING,
    ]
    assert all(item.job_id is None for item in recovered)
    assert all(item.connection_version == 2 for item in recovered)
    assert all(item.policy_snapshot == command.policy_snapshot for item in recovered)


def test_acceptance_rolls_back_with_job_transaction(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    command = _due(owner_id, now)
    job_id, operation_id = uuid4(), uuid4()
    with sessions() as session, session.begin():
        CollectionDueWindowService(session).record_due_in_transaction(command)
    with (
        sessions() as session,
        pytest.raises(RuntimeError, match="injected rollback"),
        session.begin(),
    ):
        session.execute(
            text(
                "INSERT INTO jobs (id, owner_id, operation_id, kind, configuration_ref, "
                "configuration_version, source_key, source_capability, scope, "
                "request_fingerprint, created_at, updated_at) VALUES "
                "(:id, :owner_id, :operation_id, 'keyword.search', 'coverage:test', "
                "1, 'bilibili', 'search', CAST(:scope AS jsonb), :fingerprint, :now, :now)"
            ),
            {
                "id": job_id,
                "owner_id": owner_id,
                "operation_id": operation_id,
                "fingerprint": b"c" * 32,
                "scope": '{"connection_version":2}',
                "now": now,
            },
        )
        CollectionDueWindowService(session).mark_accepted_in_transaction(
            owner_id=owner_id,
            schedule_key=command.schedule_key,
            due_at=command.due_at,
            operation_id=operation_id,
            job_id=job_id,
        )
        raise RuntimeError("injected rollback")
    with sessions() as session:
        due = CollectionDueWindowService(session).list_due(
            owner_id=owner_id, start=now, end=now + timedelta(seconds=1)
        )
        assert len(due) == 1 and due[0].admission_state is DueAdmissionState.PENDING
        assert due[0].job_id is None
        assert session.scalar(text("SELECT count(*) FROM jobs WHERE id = :id"), {"id": job_id}) == 0


def test_recovery_freezes_the_version_effective_at_each_due_point(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    command = _due(owner_id, now - timedelta(hours=12))
    assert command.policy_snapshot is not None
    disabled = command.policy_snapshot.model_copy(update={"enabled": False})

    def snapshot_at(due_at: datetime) -> tuple[int, SourceExecutionPolicy]:
        if due_at == command.due_at:
            return 2, command.policy_snapshot
        return 3, disabled

    with sessions() as session, session.begin():
        rows = CollectionDueWindowService(session).record_elapsed_in_transaction(
            first_due=command,
            interval_seconds=6 * 3600,
            through_at=now,
            snapshot_at=snapshot_at,
        )
    assert [row.connection_version for row in rows] == [2, 3, 3]
    assert [row.admission_state for row in rows] == [
        DueAdmissionState.MISSED,
        DueAdmissionState.SKIPPED,
        DueAdmissionState.SKIPPED,
    ]
    assert rows[0].policy_snapshot == command.policy_snapshot
    assert rows[1].policy_snapshot == disabled


def test_late_scan_keeps_the_latest_due_pending_for_admission(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    command = _due(owner_id, now - timedelta(minutes=1))
    with sessions() as session, session.begin():
        rows = CollectionDueWindowService(session).record_elapsed_in_transaction(
            first_due=command,
            interval_seconds=6 * 3600,
            through_at=now,
            snapshot_at=lambda _: (command.connection_version, command.policy_snapshot),
        )
    assert len(rows) == 1
    assert rows[0].due_at == command.due_at
    assert rows[0].admission_state is DueAdmissionState.PENDING


def test_utc_bounds_and_owner_scope() -> None:
    owner_id = uuid4()
    now = datetime(2026, 9, 27, 0, 0, tzinfo=UTC)
    with pytest.raises(ValueError, match="UTC"):
        CollectionDueWindowInput.model_validate(
            _due(owner_id, now).model_dump() | {"due_at": now.replace(tzinfo=None)}
        )


def test_database_rejects_skipped_due_without_a_reason(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    with sessions() as session, pytest.raises(IntegrityError), session.begin():
        session.execute(
            text(
                "INSERT INTO collection_due_windows "
                "(id, owner_id, schedule_key, source_key, capability, due_at, "
                "window_start, window_end, admission_state, recorded_at) "
                "VALUES (:id, :owner_id, :schedule_key, 'bilibili', 'search', "
                ":now, :start, :now, 'skipped', :now)"
            ),
            {
                "id": uuid4(),
                "owner_id": owner_id,
                "schedule_key": uuid4(),
                "start": now - timedelta(hours=6),
                "now": now,
            },
        )


def test_quiet_window_across_midnight_skips_each_actual_due(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    first_at = now - timedelta(hours=9)  # 23:00 in Asia/Shanghai
    base = _due(owner_id, first_at)
    assert base.policy_snapshot is not None
    command = base.model_copy(
        update={
            "policy_snapshot": base.policy_snapshot.model_copy(
                update={
                    "quiet_windows": (
                        SourceQuietWindow(timezone="Asia/Shanghai", start="22:00", end="08:00"),
                    )
                }
            )
        }
    )
    with sessions() as session, session.begin():
        rows = CollectionDueWindowService(session).record_elapsed_in_transaction(
            first_due=command,
            interval_seconds=6 * 3600,
            through_at=first_at + timedelta(hours=12),
            snapshot_at=lambda _: (command.connection_version, command.policy_snapshot),
        )
    assert [row.admission_state for row in rows] == [
        DueAdmissionState.SKIPPED,
        DueAdmissionState.SKIPPED,
        DueAdmissionState.PENDING,
    ]
    assert [row.reason for row in rows] == ["quiet", "quiet", None]


def test_due_and_execution_projection_distinguish_unknown_from_real_zero(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    pending = _due(owner_id, now)
    accepted = _due(owner_id, now + timedelta(hours=6), schedule_key=pending.schedule_key)
    job_id, operation_id = uuid4(), uuid4()
    with sessions() as session, session.begin():
        service = CollectionDueWindowService(session)
        service.record_due_in_transaction(pending)
        service.record_due_in_transaction(accepted)
        session.execute(
            text(
                "INSERT INTO jobs (id, owner_id, operation_id, kind, configuration_ref, "
                "configuration_version, source_key, source_capability, scope, "
                "request_fingerprint, created_at, updated_at) VALUES "
                "(:id, :owner_id, :operation_id, 'keyword.search', 'coverage:test', "
                "1, 'bilibili', 'search', CAST(:scope AS jsonb), :fingerprint, :now, :now)"
            ),
            {
                "id": job_id,
                "owner_id": owner_id,
                "operation_id": operation_id,
                "fingerprint": b"d" * 32,
                "scope": '{"connection_version":2}',
                "now": now,
            },
        )
        service.mark_accepted_in_transaction(
            owner_id=owner_id,
            schedule_key=accepted.schedule_key,
            due_at=accepted.due_at,
            operation_id=operation_id,
            job_id=job_id,
        )
    with sessions() as session:
        service = CollectionDueWindowService(session)
        rows = service.list_execution_facts(
            owner_id=owner_id, start=now, end=now + timedelta(hours=12)
        )
        assert service.list_due(owner_id=uuid4(), start=now, end=now + timedelta(hours=12)) == ()
    assert len(rows) == 2
    assert rows[0].requests_sent is None and rows[0].page_count is None
    assert rows[1].requests_sent == 0 and rows[1].page_count == 0
    assert rows[1].request_attempt_count == 0 and rows[1].charged_request_count == 0
    assert rows[1].request_budget_reconciled is True
    assert rows[0].has_gap and rows[1].has_gap

    with sessions() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO coverage_windows "
                "(id, owner_id, source_key, capability, target_hash, sort_key, "
                "rule_version, starts_at, ends_at, status, stop_reason, last_job_id, "
                "page_count, created_at, updated_at) VALUES "
                "(:id, :owner_id, 'bilibili', 'search', :target_hash, 'latest', "
                "1, :starts_at, :ends_at, 'partial', 'budget_exhausted', :job_id, "
                "1, :now, :now)"
            ),
            {
                "id": uuid4(),
                "owner_id": owner_id,
                "target_hash": b"b" * 32,
                "starts_at": accepted.window_start,
                "ends_at": accepted.window_end,
                "job_id": job_id,
                "now": now,
            },
        )
    with sessions() as session:
        partial = CollectionDueWindowService(session).list_execution_facts(
            owner_id=owner_id, start=now, end=now + timedelta(hours=12)
        )[1]
    assert partial.page_count == 1
    assert partial.coverage_status is CoverageWindowStatus.PARTIAL
    assert partial.stop_reason == "budget_exhausted"
    assert partial.has_gap


def test_request_attempt_and_budget_charge_reconcile_per_operation(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    command = _due(owner_id, now)
    job_id, operation_id, attempt_id = uuid4(), uuid4(), uuid4()
    with sessions() as session, session.begin():
        due = CollectionDueWindowService(session)
        due.record_due_in_transaction(command)
        session.execute(
            text(
                "INSERT INTO jobs (id, owner_id, operation_id, kind, configuration_ref, "
                "configuration_version, source_key, source_capability, scope, "
                "request_fingerprint, requests_sent, progress_stage, progress_updated_at, "
                "created_at, updated_at) VALUES "
                "(:id, :owner_id, :operation_id, 'keyword.search', 'coverage:test', "
                "1, 'bilibili', 'search', CAST(:scope AS jsonb), "
                ":fingerprint, 1, 'request', :now, :now, :now)"
            ),
            {
                "id": job_id,
                "owner_id": owner_id,
                "operation_id": operation_id,
                "scope": '{"connection_version":2}',
                "fingerprint": b"e" * 32,
                "now": now,
            },
        )
        due.mark_accepted_in_transaction(
            owner_id=owner_id,
            schedule_key=command.schedule_key,
            due_at=command.due_at,
            operation_id=operation_id,
            job_id=job_id,
        )
    with sessions() as session:
        before = CollectionDueWindowService(session).list_execution_facts(
            owner_id=owner_id, start=now, end=now + timedelta(seconds=1)
        )[0]
    assert before.requests_sent == 1
    assert before.request_attempt_count == 0
    assert before.charged_request_count is None
    assert before.request_budget_reconciled is False

    with sessions() as session:
        budget = ResourceBudgetService(session, clock=lambda: now)
        budget.save_component_policy(
            owner_id=owner_id,
            command=ComponentPolicyInput(
                component_key="collector.bilibili",
                component_version="1",
                cost_class=CostClass.LOCAL,
                enabled_for_core=True,
                terms_reference="https://example.invalid/terms",
                reviewed_at=now,
            ),
        )
        budget.save_budget_policy(
            owner_id=owner_id,
            command=BudgetPolicyInput(
                budget_key="global.coverage.requests",
                metric=BudgetMetric.NETWORK_REQUEST,
                scope_kind=BudgetScopeKind.GLOBAL,
                scope_reference=None,
                limit_units=1,
                window_seconds=60,
                window_anchor_at=now - timedelta(seconds=10),
                enabled=True,
            ),
        )
        with session.begin():
            budget.reserve_budget_in_transaction(
                owner_id=owner_id,
                command=BudgetReservationInput(
                    reservation_id=attempt_id,
                    operation_id=operation_id,
                    metric=BudgetMetric.NETWORK_REQUEST,
                    requested_units=1,
                    context=BudgetContext(source_ref="bilibili", job_ref=f"job:{job_id.hex}"),
                ),
            )
            budget.begin_attempt_in_transaction(
                owner_id=owner_id,
                command=UsageAttemptInput(
                    attempt_id=attempt_id,
                    operation_id=operation_id,
                    component_key="collector.bilibili",
                    usage_kind=UsageKind.NETWORK_REQUEST,
                    stage="search.request",
                    started_at=now,
                ),
            )
            budget.settle_budget_reservation_in_transaction(
                owner_id=owner_id, reservation_id=attempt_id, actual_units=1
            )
            budget.finish_attempt_in_transaction(
                owner_id=owner_id,
                attempt_id=attempt_id,
                outcome=UsageOutcome.SUCCEEDED,
                finished_at=now,
            )
    with sessions() as session:
        after = CollectionDueWindowService(session).list_execution_facts(
            owner_id=owner_id, start=now, end=now + timedelta(seconds=1)
        )[0]
    assert after.request_attempt_count == after.charged_request_count == 1
    assert after.request_budget_reconciled is True


def test_cross_job_same_content_is_counted_once_as_first_ingestion(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    first_job, second_job = uuid4(), uuid4()
    shared_content, new_content = uuid4(), uuid4()
    with sessions() as session, session.begin():
        for job_id in (first_job, second_job):
            session.execute(
                text(
                    "INSERT INTO jobs (id, owner_id, operation_id, kind, configuration_ref, "
                    "configuration_version, source_key, source_capability, scope, "
                    "request_fingerprint, created_at, updated_at) VALUES "
                    "(:id, :owner_id, :operation_id, 'keyword.search', 'coverage:test', "
                    "1, 'bilibili', 'search', '{}'::jsonb, :fingerprint, :now, :now)"
                ),
                {
                    "id": job_id,
                    "owner_id": owner_id,
                    "operation_id": uuid4(),
                    "fingerprint": job_id.bytes * 2,
                    "now": now,
                },
            )
        for content_id in (shared_content, new_content):
            session.execute(
                text(
                    "INSERT INTO content_records (id, owner_id, source_key, object_type, "
                    "external_id, created_at) VALUES "
                    "(:id, :owner_id, 'bilibili', 'post', :external_id, :now)"
                ),
                {"id": content_id, "owner_id": owner_id, "external_id": content_id.hex, "now": now},
            )
        for job_id, content_id, received_at in (
            (first_job, shared_content, now),
            (second_job, shared_content, now + timedelta(seconds=1)),
            (second_job, new_content, now + timedelta(seconds=1)),
        ):
            session.execute(
                text(
                    "INSERT INTO content_observations "
                    "(id, owner_id, content_id, job_id, source_operation_id, observed_at, "
                    "received_at) VALUES "
                    "(:id, :owner_id, :content_id, :job_id, :operation_id, :now, :received_at)"
                ),
                {
                    "id": uuid4(),
                    "owner_id": owner_id,
                    "content_id": content_id,
                    "job_id": job_id,
                    "operation_id": uuid4(),
                    "now": now,
                    "received_at": received_at,
                },
            )
    with sessions() as session, session.begin():
        counts = ContentService(session).collection_counts_in_transaction(
            owner_id=owner_id, job_ids=(first_job, second_job)
        )
    assert counts[0].first_ingested_count == 1
    assert counts[1].observation_count == 2
    assert counts[1].ingested_count == 2
    assert counts[1].first_ingested_count == 1
    assert counts[1].deduplicated_count == 1


def test_annotation_projection_separates_pending_failed_and_abnormal(
    coverage_context: tuple[sessionmaker[Session], UUID, datetime],
) -> None:
    sessions, owner_id, now = coverage_context
    topic_id = uuid4()
    versions = tuple(uuid4() for _ in range(4))
    contents = tuple(uuid4() for _ in range(4))
    with sessions() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO monitor_topics "
                "(id, owner_id, name, status, readiness_status, current_version, "
                "created_at, updated_at) VALUES "
                "(:id, :owner_id, 'coverage topic', 'paused', "
                "'pending_source_selection', 1, :now, :now)"
            ),
            {"id": topic_id, "owner_id": owner_id, "now": now},
        )
        session.execute(
            text(
                "INSERT INTO monitor_topic_versions "
                "(topic_id, version, created_by, match_any, match_all, exclude, created_at) "
                "VALUES (:topic_id, 1, :owner_id, '[\"coverage\"]'::jsonb, "
                "'[]'::jsonb, '[]'::jsonb, :now)"
            ),
            {"topic_id": topic_id, "owner_id": owner_id, "now": now},
        )
        for index, (content_id, version_id) in enumerate(zip(contents, versions, strict=True)):
            session.execute(
                text(
                    "INSERT INTO content_records "
                    "(id, owner_id, source_key, object_type, external_id, created_at) "
                    "VALUES (:id, :owner_id, 'bilibili', 'post', :external_id, :now)"
                ),
                {"id": content_id, "owner_id": owner_id, "external_id": str(index), "now": now},
            )
            session.execute(
                text(
                    "INSERT INTO content_versions "
                    "(id, owner_id, content_id, fingerprint, text_scope, text_origin, "
                    "title, created_at) VALUES "
                    "(:id, :owner_id, :content_id, :fingerprint, 'full', 'source', "
                    "'coverage title', :now)"
                ),
                {
                    "id": version_id,
                    "owner_id": owner_id,
                    "content_id": content_id,
                    "fingerprint": version_id.bytes * 2,
                    "now": now,
                },
            )
        for index, status in ((0, "annotated"), (1, "unanalyzed")):
            session.execute(
                text(
                    "INSERT INTO content_annotations "
                    "(id, owner_id, content_id, content_version_id, topic_id, "
                    "topic_rule_version, prompt_version, relevant, relevance_reason, "
                    "summary, viewpoints, status, created_at) VALUES "
                    "(:id, :owner_id, :content_id, :version_id, :topic_id, 1, 'v1', "
                    ":relevant, :reason, :summary, '[]'::jsonb, :status, :now)"
                ),
                {
                    "id": uuid4(),
                    "owner_id": owner_id,
                    "content_id": contents[index],
                    "version_id": versions[index],
                    "topic_id": topic_id,
                    "relevant": False if status == "annotated" else None,
                    "reason": "not relevant" if status == "annotated" else None,
                    "summary": "no match" if status == "annotated" else None,
                    "status": status,
                    "now": now,
                },
            )
        session.execute(
            text(
                "INSERT INTO jobs (id, owner_id, operation_id, kind, configuration_ref, "
                "configuration_version, scope, request_fingerprint, status, "
                "last_error_code, last_error_category, last_error_at, next_action, "
                "created_at, updated_at) VALUES "
                "(:id, :owner_id, :operation_id, 'analysis.annotate', :ref, 1, "
                "CAST(:scope AS jsonb), :fingerprint, 'failed', 'model_unavailable', "
                "'transient', :now, 'retry later', :now, :now)"
            ),
            {
                "id": uuid4(),
                "owner_id": owner_id,
                "operation_id": uuid4(),
                "ref": f"topic:{topic_id}",
                "scope": json.dumps(
                    {
                        "topic_id": str(topic_id),
                        "topic_rule_version": 1,
                        "prompt_version": "v1",
                        "content_version_ids": json.dumps([str(versions[2])]),
                    }
                ),
                "fingerprint": b"a" * 32,
                "now": now,
            },
        )
    with sessions() as session, session.begin():
        counts = AnalysisService(session).window_annotation_counts_in_transaction(
            owner_id=owner_id,
            topic_id=topic_id,
            topic_rule_version=1,
            prompt_version="v1",
            content_version_ids=versions,
        )
        other_owner = AnalysisService(session).window_annotation_counts_in_transaction(
            owner_id=uuid4(),
            topic_id=topic_id,
            topic_rule_version=1,
            prompt_version="v1",
            content_version_ids=versions,
        )
    assert (
        counts.annotated_count,
        counts.abnormal_count,
        counts.failed_count,
        counts.pending_count,
    ) == (1, 1, 1, 1)
    assert other_owner.pending_count == 4
