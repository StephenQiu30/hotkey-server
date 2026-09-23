from __future__ import annotations

import hashlib
import json
import os
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from sqlalchemy import create_engine, text
from sqlalchemy.orm import Session, sessionmaker

from connections.schemas import SourceEntryPoint
from content.schemas import PersistContentPostInput
from content.services import ContentService
from evidence.schemas import AdmittedSourcePayload, DataClass
from jobs.execution import JobExecutionService
from sources.contracts import SourceCapability, SourcePageState, SourceSort, SourceStopReason

_TARGET_HASH = hashlib.sha256(b"controlled-target").digest()


@pytest.fixture
def window_context() -> Iterator[tuple[sessionmaker[Session], UUID, UUID, datetime]]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")
    engine = create_engine(database_url)
    sessions = sessionmaker(bind=engine, expire_on_commit=False)
    owner_id, job_id, operation_id = uuid4(), uuid4(), uuid4()
    now = datetime.now(UTC).replace(microsecond=0)
    with engine.begin() as connection:
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:id, :username, 'test-only-hash', 1, :now, :now)"
            ),
            {"id": owner_id, "username": f"window-{owner_id.hex}", "now": now},
        )
        connection.execute(
            text(
                "INSERT INTO jobs "
                "(id, owner_id, operation_id, kind, configuration_ref, "
                "configuration_version, source_key, source_capability, scope, "
                "request_fingerprint, created_at, updated_at) VALUES "
                "(:id, :owner_id, :operation_id, 'monitor.collect', 'window-test', "
                "1, 'x', 'search', CAST(:scope AS jsonb), :fingerprint, :now, :now)"
            ),
            {
                "id": job_id,
                "owner_id": owner_id,
                "operation_id": operation_id,
                "fingerprint": b"w" * 32,
                "scope": '{"target_hash":"'
                + _TARGET_HASH.hex()
                + '","sort_key":"latest","rule_version":1}',
                "now": now,
            },
        )
    try:
        yield sessions, owner_id, job_id, now
    finally:
        with engine.begin() as connection:
            connection.execute(text("SET CONSTRAINTS ALL DEFERRED"))
            connection.execute(
                text("DELETE FROM content_records WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            )
            connection.execute(
                text("DELETE FROM source_connection_versions WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            )
            connection.execute(
                text("DELETE FROM identity_users WHERE id = :owner_id"),
                {"owner_id": owner_id},
            )
        engine.dispose()


def _seed_social_policy(
    session: Session, *, owner_id: UUID, now: datetime
) -> tuple[UUID, UUID, UUID]:
    connection_id, policy_id, retention_id = uuid4(), uuid4(), uuid4()
    session.execute(text("SET CONSTRAINTS ALL DEFERRED"))
    session.execute(
        text(
            "INSERT INTO source_connections "
            "(id, owner_id, source_key, status, current_version, created_at, updated_at) "
            "VALUES (:id, :owner_id, 'x', 'active', 1, :now, :now)"
        ),
        {"id": connection_id, "owner_id": owner_id, "now": now},
    )
    session.execute(
        text(
            "INSERT INTO source_connection_versions "
            "(connection_id, version, owner_id, secret_ref, created_by, created_at) "
            "VALUES (:id, 1, :owner_id, 'env:HOTKEY_X_TOKEN', :owner_id, :now)"
        ),
        {"id": connection_id, "owner_id": owner_id, "now": now},
    )
    session.execute(
        text(
            "INSERT INTO source_access_policies "
            "(id, owner_id, source_key, capability, status, enabled, access_basis, "
            "terms_reference, processing_purpose, component_name, component_version, "
            "component_license, field_purposes, reviewed_at, policy_version, "
            "created_at, updated_at) VALUES "
            "(:id, :owner_id, 'x', 'search', 'approved', true, 'official_api', "
            "'https://developer.x.com/terms', '受控窗口验证', 'controlled-collector', "
            "'1', 'MIT', CAST(:fields AS jsonb), :now, 1, :now, :now)"
        ),
        {
            "id": policy_id,
            "owner_id": owner_id,
            "fields": json.dumps(
                {
                    "object_type": "资料类型",
                    "external_id": "来源身份",
                    "published_at": "来源时间",
                    "like_count": "互动观察",
                },
                ensure_ascii=False,
            ),
            "now": now,
        },
    )
    session.execute(
        text(
            "INSERT INTO evidence_retention_policies "
            "(id, owner_id, source_policy_id, source_policy_version, data_class, "
            "requested_days, source_max_days, effective_days, policy_version, "
            "created_at, updated_at) VALUES "
            "(:id, :owner_id, :policy_id, 1, 'structured', 30, NULL, 30, 1, :now, :now)"
        ),
        {"id": retention_id, "owner_id": owner_id, "policy_id": policy_id, "now": now},
    )
    return connection_id, policy_id, retention_id


def _post_command(
    *,
    owner_id: UUID,
    job_id: UUID,
    connection_id: UUID,
    policy_id: UUID,
    retention_id: UUID,
    operation_id: UUID,
    observed_at: datetime,
    like_count: int,
) -> PersistContentPostInput:
    return PersistContentPostInput(
        job_id=job_id,
        source_operation_id=operation_id,
        connection_id=connection_id,
        connection_version=1,
        entry_point=SourceEntryPoint.MANUAL,
        component_name="controlled-collector",
        component_version="1",
        admission=AdmittedSourcePayload(
            policy_id=policy_id,
            policy_version=1,
            owner_id=owner_id,
            source_key="x",
            capability=SourceCapability.SEARCH,
            retention_policy_id=retention_id,
            retention_policy_version=1,
            data_class=DataClass.STRUCTURED,
            collected_at=observed_at,
            expires_at=observed_at + timedelta(days=30),
            fields={
                "object_type": "post",
                "external_id": "overlap-post",
                "published_at": "2026-09-22T07:00:00Z",
                "like_count": like_count,
            },
        ),
    )


def test_partial_window_blocks_contiguous_watermark_until_verified_resume(
    window_context: tuple[sessionmaker[Session], UUID, UUID, datetime],
) -> None:
    from jobs.schemas import CoverageTerminalEvidence, CoverageWindowInput
    from jobs.services import CoverageWindowService

    sessions, owner_id, job_id, now = window_context
    start = now - timedelta(hours=2)
    middle = now - timedelta(hours=1)
    first = CoverageWindowInput(
        owner_id=owner_id,
        source_key="x",
        capability=SourceCapability.SEARCH,
        target_hash=_TARGET_HASH,
        sort_key=SourceSort.LATEST,
        rule_version=1,
        starts_at=start,
        ends_at=middle,
    )
    second = first.model_copy(update={"starts_at": middle, "ends_at": now})
    with sessions() as session:
        execution = JobExecutionService(session, lease_seconds=30, clock=lambda: now)
        lease = execution.acquire(job_id=job_id, worker_id="window-test")
        windows = CoverageWindowService(session, execution=execution, clock=lambda: now)

        with session.begin():
            windows.begin_in_transaction(lease=lease, window=first)
            lease = execution.save_checkpoint_in_transaction(
                lease, sequence=1, checkpoint={"page": 1}
            )
            assert (
                windows.record_page_in_transaction(
                    lease=lease, window=first, page_state=SourcePageState.MORE
                ).status
                == "running"
            )
        with session.begin():
            lease = execution.save_checkpoint_in_transaction(
                lease, sequence=2, checkpoint={"page": 2}
            )
            assert (
                windows.record_page_in_transaction(
                    lease=lease,
                    window=first,
                    page_state=SourcePageState.PARTIAL,
                    stop_reason=SourceStopReason.BUDGET_EXHAUSTED,
                ).status
                == "partial"
            )
        with session.begin():
            windows.begin_in_transaction(lease=lease, window=second)
            lease = execution.save_checkpoint_in_transaction(
                lease, sequence=3, checkpoint={"page": 3}
            )
            assert (
                windows.record_page_in_transaction(
                    lease=lease,
                    window=second,
                    page_state=SourcePageState.COMPLETE,
                    evidence=CoverageTerminalEvidence(
                        starts_at=second.starts_at,
                        ends_at=second.ends_at,
                        sort_key=second.sort_key,
                        query_bounded=True,
                        sort_applied=True,
                        terminal_verified=True,
                    ),
                ).status
                == "confirmed"
            )
        with session.begin():
            assert windows.confirmed_through(window=first, from_at=start) == start

        with session.begin():
            windows.begin_in_transaction(lease=lease, window=first)
            lease = execution.save_checkpoint_in_transaction(
                lease, sequence=4, checkpoint={"page": 4}
            )
            assert (
                windows.record_page_in_transaction(
                    lease=lease,
                    window=first,
                    page_state=SourcePageState.COMPLETE,
                    evidence=CoverageTerminalEvidence(
                        starts_at=first.starts_at,
                        ends_at=first.ends_at,
                        sort_key=first.sort_key,
                        query_bounded=True,
                        sort_applied=True,
                        terminal_verified=True,
                    ),
                ).status
                == "confirmed"
            )
            assert windows.begin_in_transaction(lease=lease, window=first).status == "confirmed"
        with session.begin():
            assert windows.confirmed_through(window=first, from_at=start) == now


def test_terminal_page_without_range_evidence_stays_partial(
    window_context: tuple[sessionmaker[Session], UUID, UUID, datetime],
) -> None:
    from jobs.schemas import CoverageWindowInput
    from jobs.services import CoverageWindowService

    sessions, owner_id, job_id, now = window_context
    window = CoverageWindowInput(
        owner_id=owner_id,
        source_key="x",
        capability=SourceCapability.SEARCH,
        target_hash=_TARGET_HASH,
        sort_key=SourceSort.LATEST,
        rule_version=1,
        starts_at=now - timedelta(hours=1),
        ends_at=now,
    )
    with sessions() as session:
        execution = JobExecutionService(session, lease_seconds=30, clock=lambda: now)
        lease = execution.acquire(job_id=job_id, worker_id="window-test")
        windows = CoverageWindowService(session, execution=execution, clock=lambda: now)
        with session.begin():
            windows.begin_in_transaction(lease=lease, window=window)
            lease = execution.save_checkpoint_in_transaction(
                lease, sequence=1, checkpoint={"page": 1}
            )
            result = windows.record_page_in_transaction(
                lease=lease, window=window, page_state=SourcePageState.EMPTY
            )
        assert result.status == "partial"
        assert result.stop_reason == "unverified_terminal"
        with session.begin():
            assert (
                windows.confirmed_through(window=window, from_at=window.starts_at)
                == window.starts_at
            )


def test_page_and_checkpoint_roll_back_together(
    window_context: tuple[sessionmaker[Session], UUID, UUID, datetime],
) -> None:
    from jobs.schemas import CoverageTerminalEvidence, CoverageWindowInput
    from jobs.services import CoverageWindowService

    sessions, owner_id, job_id, now = window_context
    window = CoverageWindowInput(
        owner_id=owner_id,
        source_key="x",
        capability=SourceCapability.SEARCH,
        target_hash=_TARGET_HASH,
        sort_key=SourceSort.LATEST,
        rule_version=1,
        starts_at=now - timedelta(hours=1),
        ends_at=now,
    )
    evidence = CoverageTerminalEvidence(
        starts_at=window.starts_at,
        ends_at=window.ends_at,
        sort_key=window.sort_key,
        query_bounded=True,
        sort_applied=True,
        terminal_verified=True,
    )
    with sessions() as session:
        execution = JobExecutionService(session, lease_seconds=30, clock=lambda: now)
        lease = execution.acquire(job_id=job_id, worker_id="window-test")
        windows = CoverageWindowService(session, execution=execution, clock=lambda: now)
        with session.begin():
            windows.begin_in_transaction(lease=lease, window=window)
        with (
            pytest.raises(RuntimeError, match="simulate page persistence failure"),
            session.begin(),
        ):
            updated_lease = execution.save_checkpoint_in_transaction(
                lease, sequence=1, checkpoint={"page": 1}
            )
            windows.record_page_in_transaction(
                lease=updated_lease,
                window=window,
                page_state=SourcePageState.COMPLETE,
                evidence=evidence,
            )
            raise RuntimeError("simulate page persistence failure")
        with session.begin():
            assert (
                session.scalar(
                    text("SELECT checkpoint_sequence FROM jobs WHERE id = :job_id"),
                    {"job_id": job_id},
                )
                == 0
            )
            assert session.execute(
                text("SELECT status, page_count FROM coverage_windows WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            ).one() == ("running", 0)
        with session.begin():
            updated_lease = execution.save_checkpoint_in_transaction(
                lease, sequence=1, checkpoint={"page": 1}
            )
            assert (
                windows.record_page_in_transaction(
                    lease=updated_lease,
                    window=window,
                    page_state=SourcePageState.COMPLETE,
                    evidence=evidence,
                ).status
                == "confirmed"
            )


def test_window_owner_and_job_lifecycle_are_isolated(
    window_context: tuple[sessionmaker[Session], UUID, UUID, datetime],
) -> None:
    from jobs.schemas import CoverageWindowInput
    from jobs.services import CoverageWindowConflictError, CoverageWindowService

    sessions, owner_id, job_id, now = window_context
    window = CoverageWindowInput(
        owner_id=owner_id,
        source_key="x",
        capability=SourceCapability.SEARCH,
        target_hash=_TARGET_HASH,
        sort_key=SourceSort.LATEST,
        rule_version=1,
        starts_at=now - timedelta(hours=1),
        ends_at=now,
    )
    with sessions() as session:
        execution = JobExecutionService(session, lease_seconds=30, clock=lambda: now)
        lease = execution.acquire(job_id=job_id, worker_id="window-test")
        windows = CoverageWindowService(session, execution=execution, clock=lambda: now)
        with pytest.raises(CoverageWindowConflictError, match="owner"), session.begin():
            windows.begin_in_transaction(
                lease=lease,
                window=window.model_copy(update={"owner_id": uuid4()}),
            )
        with session.begin():
            windows.begin_in_transaction(lease=lease, window=window)
        with session.begin():
            session.execute(text("DELETE FROM jobs WHERE id = :job_id"), {"job_id": job_id})
        with session.begin():
            assert session.execute(
                text("SELECT status, last_job_id FROM coverage_windows WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            ).one() == ("running", None)
        replacement_job_id = uuid4()
        with session.begin():
            session.execute(
                text(
                    "INSERT INTO jobs "
                    "(id, owner_id, operation_id, kind, configuration_ref, "
                    "configuration_version, source_key, source_capability, scope, "
                    "request_fingerprint, created_at, updated_at) VALUES "
                    "(:id, :owner_id, :operation_id, 'monitor.collect', 'window-test', "
                    "1, 'x', 'search', CAST(:scope AS jsonb), :fingerprint, :now, :now)"
                ),
                {
                    "id": replacement_job_id,
                    "owner_id": owner_id,
                    "operation_id": uuid4(),
                    "scope": json.dumps(
                        {
                            "target_hash": window.target_hash.hex(),
                            "sort_key": window.sort_key.value,
                            "rule_version": window.rule_version,
                        }
                    ),
                    "fingerprint": b"r" * 32,
                    "now": now,
                },
            )
        replacement_lease = execution.acquire(
            job_id=replacement_job_id, worker_id="window-replacement"
        )
        with session.begin():
            assert (
                windows.begin_in_transaction(lease=replacement_lease, window=window).status
                == "running"
            )


def test_overlapping_windows_reuse_post_and_preserve_two_observations(
    window_context: tuple[sessionmaker[Session], UUID, UUID, datetime],
) -> None:
    from jobs.schemas import CoverageTerminalEvidence, CoverageWindowInput
    from jobs.services import CoverageWindowService

    sessions, owner_id, first_job_id, now = window_context
    second_job_id = uuid4()
    start, middle = now - timedelta(hours=2), now - timedelta(hours=1)
    first = CoverageWindowInput(
        owner_id=owner_id,
        source_key="x",
        capability=SourceCapability.SEARCH,
        target_hash=_TARGET_HASH,
        sort_key=SourceSort.LATEST,
        rule_version=1,
        starts_at=start,
        ends_at=middle,
    )
    second = first.model_copy(update={"starts_at": middle, "ends_at": now})
    with sessions() as session:
        with session.begin():
            connection_id, policy_id, retention_id = _seed_social_policy(
                session, owner_id=owner_id, now=now
            )
            session.execute(
                text(
                    "INSERT INTO jobs "
                    "(id, owner_id, operation_id, kind, configuration_ref, "
                    "configuration_version, source_key, source_capability, scope, "
                    "request_fingerprint, created_at, updated_at) VALUES "
                    "(:id, :owner_id, :operation_id, 'monitor.collect', 'window-test', "
                    "1, 'x', 'search', CAST(:scope AS jsonb), :fingerprint, :now, :now)"
                ),
                {
                    "id": second_job_id,
                    "owner_id": owner_id,
                    "operation_id": uuid4(),
                    "scope": json.dumps(
                        {
                            "target_hash": first.target_hash.hex(),
                            "sort_key": first.sort_key.value,
                            "rule_version": first.rule_version,
                        }
                    ),
                    "fingerprint": b"b" * 32,
                    "now": now,
                },
            )
        execution = JobExecutionService(session, lease_seconds=30, clock=lambda: now)
        windows = CoverageWindowService(session, execution=execution, clock=lambda: now)
        content = ContentService(session, clock=lambda: now)
        first_lease = execution.acquire(job_id=first_job_id, worker_id="window-first")
        first_command = _post_command(
            owner_id=owner_id,
            job_id=first_job_id,
            connection_id=connection_id,
            policy_id=policy_id,
            retention_id=retention_id,
            operation_id=uuid4(),
            observed_at=now - timedelta(minutes=10),
            like_count=1,
        )
        with session.begin():
            windows.begin_in_transaction(lease=first_lease, window=first)
            created = content.persist_post_in_transaction(owner_id=owner_id, command=first_command)
            first_lease = execution.save_checkpoint_in_transaction(
                first_lease, sequence=1, checkpoint={"page": 1}
            )
            windows.record_page_in_transaction(
                lease=first_lease, window=first, page_state=SourcePageState.MORE
            )
        with session.begin():
            first_lease = execution.save_checkpoint_in_transaction(
                first_lease, sequence=2, checkpoint={"page": 2}
            )
            windows.record_page_in_transaction(
                lease=first_lease,
                window=first,
                page_state=SourcePageState.PARTIAL,
                stop_reason=SourceStopReason.BUDGET_EXHAUSTED,
            )

        second_lease = execution.acquire(job_id=second_job_id, worker_id="window-second")
        second_command = _post_command(
            owner_id=owner_id,
            job_id=second_job_id,
            connection_id=connection_id,
            policy_id=policy_id,
            retention_id=retention_id,
            operation_id=uuid4(),
            observed_at=now - timedelta(minutes=5),
            like_count=2,
        )
        with session.begin():
            windows.begin_in_transaction(lease=second_lease, window=second)
            updated = content.persist_post_in_transaction(owner_id=owner_id, command=second_command)
            second_lease = execution.save_checkpoint_in_transaction(
                second_lease, sequence=1, checkpoint={"page": 1}
            )
            windows.record_page_in_transaction(
                lease=second_lease,
                window=second,
                page_state=SourcePageState.COMPLETE,
                evidence=CoverageTerminalEvidence(
                    starts_at=second.starts_at,
                    ends_at=second.ends_at,
                    sort_key=second.sort_key,
                    query_bounded=True,
                    sort_applied=True,
                    terminal_verified=True,
                ),
            )
        assert created.id == updated.id
        with session.begin():
            assert session.execute(
                text(
                    "SELECT (SELECT count(*) FROM content_records WHERE owner_id = :owner_id), "
                    "(SELECT count(*) FROM content_discoveries WHERE owner_id = :owner_id), "
                    "(SELECT count(*) FROM content_observations WHERE owner_id = :owner_id)"
                ),
                {"owner_id": owner_id},
            ).one() == (1, 2, 2)
            assert windows.confirmed_through(window=first, from_at=start) == start

        with session.begin():
            windows.begin_in_transaction(lease=first_lease, window=first)
            replayed = content.persist_post_in_transaction(owner_id=owner_id, command=first_command)
            first_lease = execution.save_checkpoint_in_transaction(
                first_lease, sequence=3, checkpoint={"page": 3}
            )
            windows.record_page_in_transaction(
                lease=first_lease,
                window=first,
                page_state=SourcePageState.COMPLETE,
                evidence=CoverageTerminalEvidence(
                    starts_at=first.starts_at,
                    ends_at=first.ends_at,
                    sort_key=first.sort_key,
                    query_bounded=True,
                    sort_applied=True,
                    terminal_verified=True,
                ),
            )
        assert replayed.latest_observation.id == created.latest_observation.id
        with session.begin():
            assert windows.confirmed_through(window=first, from_at=start) == now
            assert session.execute(
                text(
                    "SELECT (SELECT count(*) FROM content_records WHERE owner_id = :owner_id), "
                    "(SELECT count(*) FROM content_discoveries WHERE owner_id = :owner_id), "
                    "(SELECT count(*) FROM content_observations WHERE owner_id = :owner_id)"
                ),
                {"owner_id": owner_id},
            ).one() == (1, 2, 2)
