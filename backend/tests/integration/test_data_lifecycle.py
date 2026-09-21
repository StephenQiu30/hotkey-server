from __future__ import annotations

import os
from collections.abc import Iterator
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, text
from sqlalchemy.orm import Session, sessionmaker

from evidence.schemas import (
    AccessBasis,
    AccessPolicyStatus,
    SourceAccessPolicyInput,
    SourceCapability,
)
from evidence.services import SourceAccessPolicyService, SourceAccessUnavailableError


@dataclass(frozen=True, slots=True)
class LifecycleTestContext:
    engine: Engine
    sessions: sessionmaker[Session]
    owner_id: UUID
    other_owner_id: UUID


@pytest.fixture
def lifecycle_context() -> Iterator[LifecycleTestContext]:
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
                "TRUNCATE source_access_policies, processed_messages, job_attempts, "
                "outbox_messages, jobs, identity_sessions, identity_users"
            )
        )
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:owner_id, 'lifecycle-owner', 'test-only-hash', 1, now(), now())"
            ),
            {"owner_id": owner_id},
        )
    try:
        yield LifecycleTestContext(
            engine=engine,
            sessions=sessions,
            owner_id=owner_id,
            other_owner_id=other_owner_id,
        )
    finally:
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE source_access_policies, processed_messages, job_attempts, "
                    "outbox_messages, jobs, identity_sessions, identity_users"
                )
            )
        engine.dispose()


def _policy(
    *,
    status: AccessPolicyStatus,
    enabled: bool,
    reviewed_at: datetime | None = None,
    review_expires_at: datetime | None = None,
    field_purposes: dict[str, str] | None = None,
) -> SourceAccessPolicyInput:
    approved = status is AccessPolicyStatus.APPROVED
    return SourceAccessPolicyInput(
        source_key="douyin",
        capability=SourceCapability.SEARCH,
        status=status,
        enabled=enabled,
        access_basis=AccessBasis.OFFICIAL_API if approved else None,
        terms_reference=("https://open.douyin.com/platform/resource/docs" if approved else None),
        processing_purpose="发现与已配置主题相关的公开作品",
        component_name="hotkey.sources.douyin" if approved else None,
        component_version="1.0.0" if approved else None,
        component_license="project-internal" if approved else None,
        field_purposes=field_purposes or {},
        reviewed_at=reviewed_at,
        review_expires_at=review_expires_at,
    )


def test_pending_policy_rejects_payload_admission(
    lifecycle_context: LifecycleTestContext,
) -> None:
    now = datetime(2026, 9, 21, 9, tzinfo=UTC)
    with lifecycle_context.sessions() as session:
        service = SourceAccessPolicyService(session, clock=lambda: now)
        saved = service.save(
            owner_id=lifecycle_context.owner_id,
            command=_policy(status=AccessPolicyStatus.PENDING, enabled=False),
        )
        with pytest.raises(SourceAccessUnavailableError):
            service.admit_payload(
                owner_id=lifecycle_context.owner_id,
                source_key="douyin",
                capability=SourceCapability.SEARCH,
                payload={"external_id": "video-1"},
            )

    assert saved.policy_version == 1
    assert saved.enabled is False


def test_approved_policy_minimizes_payload_and_replaces_field_scope(
    lifecycle_context: LifecycleTestContext,
) -> None:
    now = datetime(2026, 9, 21, 9, tzinfo=UTC)
    expires_at = now + timedelta(days=30)
    with lifecycle_context.sessions() as session:
        service = SourceAccessPolicyService(session, clock=lambda: now)
        first = service.save(
            owner_id=lifecycle_context.owner_id,
            command=_policy(
                status=AccessPolicyStatus.APPROVED,
                enabled=True,
                reviewed_at=now,
                review_expires_at=expires_at,
                field_purposes={
                    "external_id": "稳定识别作品",
                    "text": "核对主题相关性",
                },
            ),
        )
        admitted = service.admit_payload(
            owner_id=lifecycle_context.owner_id,
            source_key="douyin",
            capability=SourceCapability.SEARCH,
            payload={
                "external_id": "video-1",
                "text": "公开正文",
                "author_email": "not-needed@example.com",
                "access_token": "must-not-survive",
            },
        )
        second = service.save(
            owner_id=lifecycle_context.owner_id,
            command=_policy(
                status=AccessPolicyStatus.APPROVED,
                enabled=True,
                reviewed_at=now,
                review_expires_at=expires_at,
                field_purposes={"external_id": "稳定识别作品"},
            ),
        )

    assert admitted.policy_id == first.id
    assert admitted.policy_version == 1
    assert admitted.fields == {"external_id": "video-1", "text": "公开正文"}
    assert second.id == first.id
    assert second.policy_version == 2
    assert second.field_purposes == {"external_id": "稳定识别作品"}
    with lifecycle_context.engine.connect() as connection:
        stored = connection.execute(
            text(
                "SELECT count(*), max(policy_version), max(field_purposes::text) "
                "FROM source_access_policies WHERE owner_id = :owner_id"
            ),
            {"owner_id": lifecycle_context.owner_id},
        ).one()
    assert tuple(stored) == (1, 2, '{"external_id": "稳定识别作品"}')


def test_expired_or_other_owner_policy_cannot_admit_payload(
    lifecycle_context: LifecycleTestContext,
) -> None:
    clock = [datetime(2026, 9, 21, 9, tzinfo=UTC)]
    with lifecycle_context.sessions() as session:
        service = SourceAccessPolicyService(session, clock=lambda: clock[0])
        service.save(
            owner_id=lifecycle_context.owner_id,
            command=_policy(
                status=AccessPolicyStatus.APPROVED,
                enabled=True,
                reviewed_at=clock[0],
                review_expires_at=clock[0] + timedelta(hours=1),
                field_purposes={"external_id": "稳定识别作品"},
            ),
        )
        with pytest.raises(SourceAccessUnavailableError):
            service.admit_payload(
                owner_id=lifecycle_context.other_owner_id,
                source_key="douyin",
                capability=SourceCapability.SEARCH,
                payload={"external_id": "video-1"},
            )

        clock[0] += timedelta(hours=2)
        with pytest.raises(SourceAccessUnavailableError):
            service.admit_payload(
                owner_id=lifecycle_context.owner_id,
                source_key="douyin",
                capability=SourceCapability.SEARCH,
                payload={"external_id": "video-1"},
            )
