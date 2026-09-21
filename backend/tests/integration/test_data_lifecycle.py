from __future__ import annotations

import os
from collections.abc import Iterator
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from io import BytesIO
from uuid import UUID, uuid4

import pytest
from minio import Minio
from minio.error import S3Error
from redis import Redis
from sqlalchemy import Engine, create_engine, text
from sqlalchemy.orm import Session, sessionmaker

from evidence.adapters.cache import RedisCacheCleanup
from evidence.adapters.minio import MinioObjectCleanup
from evidence.schemas import (
    AccessBasis,
    AccessPolicyStatus,
    CleanupTargetKind,
    CleanupTargetSpec,
    DataClass,
    DeletionReason,
    RetentionPolicyInput,
    SourceAccessPolicyInput,
)
from evidence.services import (
    CleanupProcessor,
    LifecycleService,
    ResourceUnavailableError,
    RetentionPolicyService,
    SourceAccessPolicyService,
    SourceAccessUnavailableError,
)
from sources.contracts import SourceCapability


@dataclass(frozen=True, slots=True)
class LifecycleTestContext:
    engine: Engine
    sessions: sessionmaker[Session]
    owner_id: UUID
    other_owner_id: UUID


@dataclass(frozen=True, slots=True)
class OnlineStoreContext:
    redis: Redis
    minio: Minio
    bucket: str


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
                "TRUNCATE provenance_manifest_inputs, provenance_manifests, "
                "evidence_cleanup_targets, evidence_deletions, "
                "evidence_resources, evidence_retention_policies, source_access_policies, "
                "resource_budget_reservations, resource_budget_windows, "
                "resource_budget_policies, resource_usage_attempts, "
                "resource_component_policies, "
                "job_stage_attempts, processed_messages, job_attempts, outbox_messages, jobs, "
                "monitor_topic_versions, monitor_topics, identity_sessions, identity_users"
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
                    "TRUNCATE provenance_manifest_inputs, provenance_manifests, "
                    "evidence_cleanup_targets, evidence_deletions, "
                    "evidence_resources, evidence_retention_policies, source_access_policies, "
                    "resource_budget_reservations, resource_budget_windows, "
                    "resource_budget_policies, resource_usage_attempts, "
                    "resource_component_policies, "
                    "job_stage_attempts, processed_messages, job_attempts, outbox_messages, jobs, "
                    "monitor_topic_versions, monitor_topics, identity_sessions, identity_users"
                )
            )
        engine.dispose()


@pytest.fixture
def online_stores() -> Iterator[OnlineStoreContext]:
    redis_url = os.getenv("HOTKEY_TEST_REDIS_URL")
    endpoint = os.getenv("HOTKEY_TEST_MINIO_ENDPOINT")
    access_key = os.getenv("HOTKEY_TEST_MINIO_ACCESS_KEY")
    secret_key = os.getenv("HOTKEY_TEST_MINIO_SECRET_KEY")
    bucket = os.getenv("HOTKEY_TEST_MINIO_BUCKET")
    if not all((redis_url, endpoint, access_key, secret_key, bucket)):
        pytest.skip("Redis and MinIO test settings are required for online cleanup tests")

    redis = Redis.from_url(redis_url)
    minio = Minio(
        endpoint,
        access_key=access_key,
        secret_key=secret_key,
        secure=os.getenv("HOTKEY_TEST_MINIO_SECURE", "false").lower() == "true",
    )
    assert minio.bucket_exists(bucket)
    try:
        yield OnlineStoreContext(redis=redis, minio=minio, bucket=bucket)
    finally:
        redis.close()


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
                data_class=DataClass.STRUCTURED,
                collected_at=now,
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
        retention = RetentionPolicyService(session, clock=lambda: now).save(
            owner_id=lifecycle_context.owner_id,
            command=RetentionPolicyInput(
                source_policy_id=first.id,
                data_class=DataClass.STRUCTURED,
                requested_days=30,
                source_max_days=7,
            ),
        )
        admitted = service.admit_payload(
            owner_id=lifecycle_context.owner_id,
            source_key="douyin",
            capability=SourceCapability.SEARCH,
            data_class=DataClass.STRUCTURED,
            collected_at=now,
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
    assert admitted.retention_policy_id == retention.id
    assert admitted.retention_policy_version == 1
    assert admitted.expires_at == now + timedelta(days=7)
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
        access = service.save(
            owner_id=lifecycle_context.owner_id,
            command=_policy(
                status=AccessPolicyStatus.APPROVED,
                enabled=True,
                reviewed_at=clock[0],
                review_expires_at=clock[0] + timedelta(hours=1),
                field_purposes={"external_id": "稳定识别作品"},
            ),
        )
        RetentionPolicyService(session, clock=lambda: clock[0]).save(
            owner_id=lifecycle_context.owner_id,
            command=RetentionPolicyInput(
                source_policy_id=access.id,
                data_class=DataClass.STRUCTURED,
                requested_days=30,
                source_max_days=30,
            ),
        )
        with pytest.raises(SourceAccessUnavailableError):
            service.admit_payload(
                owner_id=lifecycle_context.other_owner_id,
                source_key="douyin",
                capability=SourceCapability.SEARCH,
                data_class=DataClass.STRUCTURED,
                collected_at=clock[0],
                payload={"external_id": "video-1"},
            )

        clock[0] += timedelta(hours=2)
        with pytest.raises(SourceAccessUnavailableError):
            service.admit_payload(
                owner_id=lifecycle_context.owner_id,
                source_key="douyin",
                capability=SourceCapability.SEARCH,
                data_class=DataClass.STRUCTURED,
                collected_at=clock[0],
                payload={"external_id": "video-1"},
            )


def test_retention_expiry_blocks_reads_before_cleanup(
    lifecycle_context: LifecycleTestContext,
) -> None:
    clock = [datetime(2026, 9, 21, 9, tzinfo=UTC)]
    resource_id = uuid4()
    with lifecycle_context.sessions() as session:
        access = SourceAccessPolicyService(session, clock=lambda: clock[0]).save(
            owner_id=lifecycle_context.owner_id,
            command=_policy(
                status=AccessPolicyStatus.APPROVED,
                enabled=True,
                reviewed_at=clock[0],
                review_expires_at=clock[0] + timedelta(days=30),
                field_purposes={"external_id": "稳定识别作品"},
            ),
        )
        retention_service = RetentionPolicyService(session, clock=lambda: clock[0])
        retention_service.save(
            owner_id=lifecycle_context.owner_id,
            command=RetentionPolicyInput(
                source_policy_id=access.id,
                data_class=DataClass.STRUCTURED,
                requested_days=30,
                source_max_days=None,
            ),
        )
        admission = SourceAccessPolicyService(session, clock=lambda: clock[0]).admit_payload(
            owner_id=lifecycle_context.owner_id,
            source_key="douyin",
            capability=SourceCapability.SEARCH,
            data_class=DataClass.STRUCTURED,
            collected_at=clock[0],
            payload={"external_id": "video-expiring"},
        )
        lifecycle = LifecycleService(session, clock=lambda: clock[0])
        with pytest.raises(ResourceUnavailableError):
            lifecycle.track_resource(
                owner_id=lifecycle_context.owner_id,
                resource_type="content_item",
                resource_id=uuid4(),
                admission=admission.model_copy(
                    update={"expires_at": admission.expires_at + timedelta(days=1)}
                ),
                cleanup_targets=[],
            )
        tracked = lifecycle.track_resource(
            owner_id=lifecycle_context.owner_id,
            resource_type="content_item",
            resource_id=resource_id,
            admission=admission,
            cleanup_targets=[],
        )
        lifecycle.assert_readable(
            owner_id=lifecycle_context.owner_id,
            resource_type="content_item",
            resource_id=resource_id,
        )

        shortened = retention_service.save(
            owner_id=lifecycle_context.owner_id,
            command=RetentionPolicyInput(
                source_policy_id=access.id,
                data_class=DataClass.STRUCTURED,
                requested_days=30,
                source_max_days=1,
            ),
        )
        current = lifecycle.assert_readable(
            owner_id=lifecycle_context.owner_id,
            resource_type="content_item",
            resource_id=resource_id,
        )

        clock[0] += timedelta(days=2)
        with pytest.raises(ResourceUnavailableError):
            lifecycle.assert_readable(
                owner_id=lifecycle_context.owner_id,
                resource_type="content_item",
                resource_id=resource_id,
            )
        deletions = lifecycle.expire_due(limit=10)

    assert tracked.expires_at == datetime(2026, 10, 21, 9, tzinfo=UTC)
    assert shortened.policy_version == 2
    assert current.expires_at == datetime(2026, 9, 22, 9, tzinfo=UTC)
    assert len(deletions) == 1
    assert deletions[0].reason is DeletionReason.RETENTION_EXPIRED
    assert deletions[0].status.value == "completed"


def test_deletion_barrier_precedes_real_redis_and_minio_cleanup(
    lifecycle_context: LifecycleTestContext,
    online_stores: OnlineStoreContext,
) -> None:
    now = datetime(2026, 9, 21, 9, tzinfo=UTC)
    resource_id = uuid4()
    redis_key = f"hotkey:test:lifecycle:{resource_id}"
    object_name = f"tests/lifecycle/{resource_id}.txt"
    online_stores.redis.set(redis_key, b"must-be-deleted")
    online_stores.minio.put_object(
        online_stores.bucket,
        object_name,
        BytesIO(b"must-be-deleted"),
        length=len(b"must-be-deleted"),
    )
    try:
        with lifecycle_context.sessions() as session:
            access = SourceAccessPolicyService(session, clock=lambda: now).save(
                owner_id=lifecycle_context.owner_id,
                command=_policy(
                    status=AccessPolicyStatus.APPROVED,
                    enabled=True,
                    reviewed_at=now,
                    review_expires_at=now + timedelta(days=30),
                    field_purposes={"external_id": "稳定识别作品"},
                ),
            )
            RetentionPolicyService(session, clock=lambda: now).save(
                owner_id=lifecycle_context.owner_id,
                command=RetentionPolicyInput(
                    source_policy_id=access.id,
                    data_class=DataClass.STRUCTURED,
                    requested_days=30,
                    source_max_days=None,
                ),
            )
            admission = SourceAccessPolicyService(session, clock=lambda: now).admit_payload(
                owner_id=lifecycle_context.owner_id,
                source_key="douyin",
                capability=SourceCapability.SEARCH,
                data_class=DataClass.STRUCTURED,
                collected_at=now,
                payload={"external_id": "video-delete"},
            )
            lifecycle = LifecycleService(session, clock=lambda: now)
            lifecycle.track_resource(
                owner_id=lifecycle_context.owner_id,
                resource_type="content_item",
                resource_id=resource_id,
                admission=admission,
                cleanup_targets=[
                    CleanupTargetSpec(
                        kind=CleanupTargetKind.REDIS_CACHE,
                        reference=redis_key,
                    ),
                    CleanupTargetSpec(
                        kind=CleanupTargetKind.MINIO_OBJECT,
                        reference=object_name,
                    ),
                ],
            )
            operation_id = uuid4()
            deletion = lifecycle.request_deletion(
                owner_id=lifecycle_context.owner_id,
                operation_id=operation_id,
                resource_type="content_item",
                resource_id=resource_id,
                reason=DeletionReason.USER_REQUEST,
            )
            duplicate = lifecycle.request_deletion(
                owner_id=lifecycle_context.owner_id,
                operation_id=operation_id,
                resource_type="content_item",
                resource_id=resource_id,
                reason=DeletionReason.USER_REQUEST,
            )
            assert duplicate.id == deletion.id
            with pytest.raises(ResourceUnavailableError):
                lifecycle.assert_readable(
                    owner_id=lifecycle_context.other_owner_id,
                    resource_type="content_item",
                    resource_id=resource_id,
                )
            with pytest.raises(ResourceUnavailableError):
                lifecycle.assert_readable(
                    owner_id=lifecycle_context.owner_id,
                    resource_type="content_item",
                    resource_id=resource_id,
                )

        assert online_stores.redis.exists(redis_key) == 1
        online_stores.minio.stat_object(online_stores.bucket, object_name)
        result = CleanupProcessor(
            lifecycle_context.sessions,
            handlers={
                CleanupTargetKind.REDIS_CACHE: RedisCacheCleanup(online_stores.redis),
                CleanupTargetKind.MINIO_OBJECT: MinioObjectCleanup(
                    online_stores.minio,
                    online_stores.bucket,
                ),
            },
            clock=lambda: now,
        ).process_due(limit=10)

        assert result.succeeded == 2
        assert result.failed == 0
        assert online_stores.redis.exists(redis_key) == 0
        with pytest.raises(S3Error) as missing:
            online_stores.minio.stat_object(online_stores.bucket, object_name)
        assert missing.value.code == "NoSuchKey"
        with lifecycle_context.sessions() as session:
            completed = LifecycleService(session, clock=lambda: now).get_deletion(
                owner_id=lifecycle_context.owner_id,
                deletion_id=deletion.id,
            )
        assert completed.status.value == "completed"
        assert completed.completed_at == now
    finally:
        online_stores.redis.delete(redis_key)
        online_stores.minio.remove_object(online_stores.bucket, object_name)


def test_failed_cleanup_is_retried_without_lifting_the_barrier(
    lifecycle_context: LifecycleTestContext,
    online_stores: OnlineStoreContext,
) -> None:
    clock = [datetime(2026, 9, 21, 9, tzinfo=UTC)]
    resource_id = uuid4()
    redis_key = f"hotkey:test:lifecycle:{resource_id}"
    online_stores.redis.set(redis_key, b"retry-me")
    try:
        with lifecycle_context.sessions() as session:
            access = SourceAccessPolicyService(session, clock=lambda: clock[0]).save(
                owner_id=lifecycle_context.owner_id,
                command=_policy(
                    status=AccessPolicyStatus.APPROVED,
                    enabled=True,
                    reviewed_at=clock[0],
                    review_expires_at=clock[0] + timedelta(days=30),
                    field_purposes={"external_id": "稳定识别作品"},
                ),
            )
            RetentionPolicyService(session, clock=lambda: clock[0]).save(
                owner_id=lifecycle_context.owner_id,
                command=RetentionPolicyInput(
                    source_policy_id=access.id,
                    data_class=DataClass.STRUCTURED,
                    requested_days=30,
                    source_max_days=None,
                ),
            )
            admission = SourceAccessPolicyService(
                session,
                clock=lambda: clock[0],
            ).admit_payload(
                owner_id=lifecycle_context.owner_id,
                source_key="douyin",
                capability=SourceCapability.SEARCH,
                data_class=DataClass.STRUCTURED,
                collected_at=clock[0],
                payload={"external_id": "video-retry"},
            )
            lifecycle = LifecycleService(session, clock=lambda: clock[0])
            lifecycle.track_resource(
                owner_id=lifecycle_context.owner_id,
                resource_type="content_item",
                resource_id=resource_id,
                admission=admission,
                cleanup_targets=[
                    CleanupTargetSpec(
                        kind=CleanupTargetKind.REDIS_CACHE,
                        reference=redis_key,
                    )
                ],
            )
            lifecycle.request_deletion(
                owner_id=lifecycle_context.owner_id,
                operation_id=uuid4(),
                resource_type="content_item",
                resource_id=resource_id,
                reason=DeletionReason.USER_REQUEST,
            )

        def fail_once(_reference: str) -> None:
            raise TimeoutError("controlled cleanup failure")

        first = CleanupProcessor(
            lifecycle_context.sessions,
            handlers={CleanupTargetKind.REDIS_CACHE: fail_once},
            clock=lambda: clock[0],
        ).process_due(limit=10)
        assert first.failed == 1
        assert online_stores.redis.exists(redis_key) == 1

        clock[0] += timedelta(minutes=2)
        second = CleanupProcessor(
            lifecycle_context.sessions,
            handlers={
                CleanupTargetKind.REDIS_CACHE: RedisCacheCleanup(online_stores.redis),
            },
            clock=lambda: clock[0],
        ).process_due(limit=10)
        assert second.succeeded == 1
        assert online_stores.redis.exists(redis_key) == 0
        with (
            lifecycle_context.sessions() as session,
            pytest.raises(ResourceUnavailableError),
        ):
            LifecycleService(session, clock=lambda: clock[0]).assert_readable(
                owner_id=lifecycle_context.owner_id,
                resource_type="content_item",
                resource_id=resource_id,
            )
    finally:
        online_stores.redis.delete(redis_key)
