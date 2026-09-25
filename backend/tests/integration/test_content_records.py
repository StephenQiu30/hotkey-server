from __future__ import annotations

import json
import os
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text

from connections.schemas import SourceEntryPoint
from connections.services import SourceConnectionService
from content.schemas import (
    ContentVisibilityBasis,
    ContentVisibilityStatus,
    PersistContentPostInput,
    RecordContentVisibilityInput,
)
from content.services import ContentObservationCleanup, ContentService
from core.config import Settings
from core.errors import ApplicationError
from evidence.schemas import (
    AdmittedSourcePayload,
    CleanupTargetKind,
    DataClass,
    DeletionReason,
    DeletionStatus,
)
from evidence.services import CleanupProcessor, LifecycleService
from main import create_app
from sources.contracts import SourceCapability

_BOOTSTRAP_TOKEN = "content-records-isolated-bootstrap-token"
_PASSWORD = "correct horse battery staple"
_TRUNCATE = (
    "TRUNCATE content_version_relations, content_visibility_observations, "
    "content_observations, content_versions, "
    "content_discoveries, content_threads, content_records, "
    "source_capability_evidence, source_connection_versions, source_connections, "
    "provenance_manifest_inputs, provenance_manifests, evidence_cleanup_targets, "
    "evidence_deletions, evidence_resources, evidence_retention_policies, "
    "source_access_policies, resource_budget_reservations, resource_budget_windows, "
    "resource_budget_policies, resource_usage_attempts, resource_component_policies, "
    "job_stage_attempts, processed_messages, job_attempts, "
    "ai_calls, content_annotations, reports, monitor_schedules, outbox_messages, coverage_windows, "
    "jobs, followed_account_aliases, followed_accounts, "
    "monitor_topic_versions, monitor_topics, "
    "identity_sessions, identity_users"
)


@pytest.fixture
def content_client() -> Iterator[TestClient]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")
    settings = Settings(
        environment="test",
        log_level="WARNING",
        database_url=database_url,
        bootstrap_token=_BOOTSTRAP_TOKEN,
    )
    engine = create_engine(database_url)
    with engine.begin() as connection:
        connection.execute(text(_TRUNCATE))
    try:
        with TestClient(create_app(settings)) as client:
            yield client
    finally:
        with engine.begin() as connection:
            connection.execute(text(_TRUNCATE))
        engine.dispose()


def _initialize(client: TestClient) -> UUID:
    response = client.post(
        "/api/identity/initialize",
        headers={
            "X-HotKey-Bootstrap-Token": _BOOTSTRAP_TOKEN,
            "X-HotKey-CSRF": "1",
        },
        json={"username": "content-owner", "password": _PASSWORD},
    )
    assert response.status_code == 201
    return UUID(response.json()["user"]["id"])


def _seed_context(client: TestClient, owner_id: UUID) -> tuple[UUID, UUID, UUID, UUID, UUID]:
    connection_id = uuid4()
    policy_id = uuid4()
    retention_id = uuid4()
    job_ids = (uuid4(), uuid4())
    now = datetime.now(UTC)
    factory = client.app.state.session_factory
    with factory() as session, session.begin():
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
        fields = {
            "object_type": "作品类型",
            "external_id": "作品身份",
            "canonical_url": "原文入口",
            "author_external_id": "公开作者身份",
            "published_at": "发布时间",
            "like_count": "点赞观察",
            "comment_count": "评论观察",
            "repost_count": "转发观察",
            "view_count": "浏览观察",
            "play_count": "播放观察",
            "danmaku_count": "弹幕观察",
            "text_scope": "正文完整度",
            "text_origin": "正文来源",
            "text_origin_ref": "机器提取依据",
            "title": "作品标题",
            "body": "作品正文",
            "truncation_reason": "截断原因",
            "quote_target_external_id": "引用目标身份",
            "quote_target_native_scope": "引用目标作用域",
            "quote_target_author_external_id": "引用目标作者",
            "repost_target_external_id": "转帖目标身份",
            "repost_target_native_scope": "转帖目标作用域",
            "repost_target_author_external_id": "转帖目标作者",
        }
        session.execute(
            text(
                "INSERT INTO source_access_policies "
                "(id, owner_id, source_key, capability, status, enabled, access_basis, "
                "terms_reference, processing_purpose, component_name, component_version, "
                "component_license, field_purposes, reviewed_at, review_expires_at, "
                "policy_version, created_at, updated_at) VALUES "
                "(:id, :owner_id, 'x', 'search', 'approved', true, 'official_api', "
                "'https://developer.x.com/terms', '受控作品资料验证', 'controlled-collector', "
                "'1', 'MIT', CAST(:fields AS jsonb), :now, NULL, 1, :now, :now)"
            ),
            {
                "id": policy_id,
                "owner_id": owner_id,
                "fields": json.dumps(fields, ensure_ascii=False),
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
            {
                "id": retention_id,
                "owner_id": owner_id,
                "policy_id": policy_id,
                "now": now,
            },
        )
        for index, job_id in enumerate(job_ids, start=1):
            session.execute(
                text(
                    "INSERT INTO jobs "
                    "(id, owner_id, operation_id, kind, configuration_ref, "
                    "configuration_version, source_key, source_capability, scope, "
                    "request_fingerprint, status, created_at, updated_at) VALUES "
                    "(:id, :owner_id, :operation_id, 'monitor.collect', :configuration_ref, "
                    "1, 'x', 'search', '{}'::jsonb, :fingerprint, 'queued', :now, :now)"
                ),
                {
                    "id": job_id,
                    "owner_id": owner_id,
                    "operation_id": uuid4(),
                    "configuration_ref": f"topic:controlled-{index}",
                    "fingerprint": bytes([index]) * 32,
                    "now": now,
                },
            )
    return connection_id, policy_id, retention_id, *job_ids


def _command(
    *,
    owner_id: UUID,
    connection_id: UUID,
    policy_id: UUID,
    retention_id: UUID,
    job_id: UUID,
    operation_id: UUID,
    observed_at: datetime,
    like_count: int = 0,
    external_id: str = "post-001",
    extra_fields: dict[str, object] | None = None,
) -> PersistContentPostInput:
    return PersistContentPostInput(
        job_id=job_id,
        source_operation_id=operation_id,
        connection_id=connection_id,
        connection_version=1,
        entry_point=SourceEntryPoint.MANUAL,
        component_name="controlled-collector",
        component_version="1",
        native_scope=None,
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
                "external_id": external_id,
                "canonical_url": f"https://example.invalid/posts/{external_id}",
                "author_external_id": "author-1",
                "published_at": "2026-09-22T07:00:00Z",
                "like_count": like_count,
                "comment_count": None,
                "repost_count": 0,
                "view_count": None,
                "play_count": None,
                "danmaku_count": None,
                **(extra_fields or {}),
            },
        ),
    )


def test_content_versions_preserve_scope_provenance_relations_and_snapshot_time(
    content_client: TestClient,
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, first_job_id, second_job_id = _seed_context(
        content_client, owner_id
    )
    base_time = datetime.now(UTC) - timedelta(minutes=3)
    factory = content_client.app.state.session_factory

    target_command = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=first_job_id,
        operation_id=uuid4(),
        observed_at=base_time,
        external_id="post-target",
        extra_fields={
            "text_scope": "full",
            "text_origin": "source",
            "title": "目标作品",
            "body": "目标正文",
            "published_at": "2026-09-22T06:59:59.123456Z",
        },
    )
    subject_fields = {
        "author_external_id": "author-subject",
        "published_at": "2026-09-22T07:00:00.123Z",
        "text_scope": "summary",
        "text_origin": "source",
        "title": "摘要标题",
        "body": "<b>来源摘要</b>",
        "quote_target_external_id": "post-missing",
        "quote_target_author_external_id": "author-quote",
        "repost_target_external_id": "post-target",
        "repost_target_author_external_id": "author-target",
    }
    first_operation = uuid4()
    subject = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=first_job_id,
        operation_id=first_operation,
        observed_at=base_time + timedelta(minutes=1),
        external_id="post-subject",
        extra_fields=subject_fields,
    )
    later = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=second_job_id,
        operation_id=uuid4(),
        observed_at=base_time + timedelta(minutes=2),
        external_id="post-subject",
        extra_fields=subject_fields,
    )

    with factory() as session:
        service = ContentService(session)
        target = service.persist_post(owner_id=owner_id, command=target_command)
        created = service.persist_post(owner_id=owner_id, command=subject)
        replayed = service.persist_post(owner_id=owner_id, command=subject)
        updated = service.persist_post(owner_id=owner_id, command=later)

    assert created.latest_observation.id == replayed.latest_observation.id
    assert created.latest_observation.content_version is not None
    assert updated.latest_observation.content_version is not None
    assert (
        created.latest_observation.content_version.id
        == updated.latest_observation.content_version.id
    )
    detail = content_client.get(f"/api/contents/{created.id}")
    assert detail.status_code == 200, detail.json()
    observation = detail.json()["latest_observation"]
    assert observation["observed_at"] != created.latest_observation.observed_at.isoformat()
    assert observation["published_at_fractional_digits"] == 3
    assert observation["author_external_id"] == "author-subject"
    assert observation["content_version"] == {
        "id": str(created.latest_observation.content_version.id),
        "text_scope": "summary",
        "text_origin": "source",
        "text_origin_ref": None,
        "title": "摘要标题",
        "body": "<b>来源摘要</b>",
        "truncation_reason": None,
        "relations": [
            {
                "relation_type": "quote",
                "target_native_scope": None,
                "target_external_id": "post-missing",
                "target_author_external_id": "author-quote",
                "target_content_id": None,
            },
            {
                "relation_type": "repost",
                "target_native_scope": None,
                "target_external_id": "post-target",
                "target_author_external_id": "author-target",
                "target_content_id": str(target.id),
            },
        ],
    }
    with factory() as session:
        counts = session.execute(
            text(
                "SELECT (SELECT count(*) FROM content_records), "
                "(SELECT count(*) FROM content_versions), "
                "(SELECT count(*) FROM content_version_relations), "
                "(SELECT count(*) FROM content_observations)"
            )
        ).one()
    assert tuple(counts) == (2, 2, 2, 3)


def test_edit_and_visibility_history_keep_last_success_across_failures_and_late_data(
    content_client: TestClient,
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, job_id, _ = _seed_context(content_client, owner_id)
    base_time = datetime.now(UTC) - timedelta(minutes=10)
    factory = content_client.app.state.session_factory
    first = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=job_id,
        operation_id=uuid4(),
        observed_at=base_time,
        extra_fields={
            "text_scope": "full",
            "text_origin": "source",
            "body": "第一版正文",
        },
    )
    second = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=job_id,
        operation_id=uuid4(),
        observed_at=base_time + timedelta(minutes=2),
        extra_fields={
            "text_scope": "full",
            "text_origin": "source",
            "body": "第二版正文",
        },
    )

    with factory() as session:
        created = ContentService(
            session, clock=lambda: base_time + timedelta(seconds=30)
        ).persist_post(owner_id=owner_id, command=first)
        updated = ContentService(
            session, clock=lambda: base_time + timedelta(minutes=2, seconds=30)
        ).persist_post(owner_id=owner_id, command=second)
        service = ContentService(session, clock=lambda: base_time + timedelta(minutes=6))
        service.record_visibility(
            owner_id=owner_id,
            command=RecordContentVisibilityInput(
                content_id=created.id,
                job_id=job_id,
                source_operation_id=uuid4(),
                observed_at=base_time + timedelta(minutes=1),
                status=ContentVisibilityStatus.DELETED,
                basis=ContentVisibilityBasis.HTTP_GONE,
            ),
        )
        after_late_delete = service.get_content(owner_id=owner_id, content_id=created.id)
        service.record_visibility(
            owner_id=owner_id,
            command=RecordContentVisibilityInput(
                content_id=created.id,
                job_id=job_id,
                source_operation_id=uuid4(),
                observed_at=base_time + timedelta(minutes=3),
                status=ContentVisibilityStatus.TRANSIENT_FAILURE,
                basis=ContentVisibilityBasis.TIMEOUT,
            ),
        )
        after_timeout = service.get_content(owner_id=owner_id, content_id=created.id)
        service.record_visibility(
            owner_id=owner_id,
            command=RecordContentVisibilityInput(
                content_id=created.id,
                job_id=job_id,
                source_operation_id=uuid4(),
                observed_at=base_time + timedelta(minutes=4),
                status=ContentVisibilityStatus.UNKNOWN,
                basis=ContentVisibilityBasis.NOT_FOUND,
            ),
        )
        service.record_visibility(
            owner_id=owner_id,
            command=RecordContentVisibilityInput(
                content_id=created.id,
                job_id=job_id,
                source_operation_id=uuid4(),
                observed_at=base_time + timedelta(minutes=5),
                status=ContentVisibilityStatus.DELETED,
                basis=ContentVisibilityBasis.SOURCE_TOMBSTONE,
            ),
        )

    assert created.id == updated.id
    assert after_late_delete.current_visibility is not None
    assert after_late_delete.current_visibility.status is ContentVisibilityStatus.VISIBLE
    assert after_timeout.current_visibility is not None
    assert after_timeout.current_visibility.status is ContentVisibilityStatus.TRANSIENT_FAILURE
    assert after_timeout.latest_observation.content_version is not None
    assert after_timeout.latest_observation.content_version.body == "第二版正文"

    response = content_client.get(f"/api/contents/{created.id}")
    assert response.status_code == 200, response.json()
    detail = response.json()
    assert detail["current_visibility"]["status"] == "deleted"
    assert detail["current_visibility"]["basis"] == "source_tombstone"
    assert detail["latest_observation"]["content_version"]["body"] == "第二版正文"
    assert [item["content_version"]["body"] for item in detail["version_history"]] == [
        "第二版正文",
        "第一版正文",
    ]
    assert [item["status"] for item in detail["visibility_history"]] == [
        "deleted",
        "unknown",
        "transient_failure",
        "visible",
        "deleted",
        "visible",
    ]


def test_lifecycle_cleanup_removes_postgres_content_and_blocks_exact_replay(
    content_client: TestClient,
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, job_id, _ = _seed_context(content_client, owner_id)
    observed_at = datetime.now(UTC) - timedelta(minutes=1)
    command = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=job_id,
        operation_id=uuid4(),
        observed_at=observed_at,
        extra_fields={
            "text_scope": "full",
            "text_origin": "source",
            "body": "待清理正文",
        },
    )
    factory = content_client.app.state.session_factory
    with factory() as session:
        created = ContentService(session).persist_post(owner_id=owner_id, command=command)
        deletion = LifecycleService(session).request_deletion(
            owner_id=owner_id,
            operation_id=uuid4(),
            resource_type="content_observation",
            resource_id=created.latest_observation.id,
            reason=DeletionReason.USER_REQUEST,
        )

    assert deletion.status is DeletionStatus.PENDING
    assert deletion.target_count == 1
    result = CleanupProcessor(
        factory,
        handlers={
            CleanupTargetKind.POSTGRES_CONTENT_OBSERVATION: ContentObservationCleanup(factory)
        },
    ).process_due(limit=1)
    assert result.succeeded == 1
    assert result.failed == 0

    with factory() as session:
        completed = LifecycleService(session).get_deletion(
            owner_id=owner_id, deletion_id=deletion.id
        )
        counts = session.execute(
            text(
                "SELECT (SELECT count(*) FROM content_records), "
                "(SELECT count(*) FROM content_discoveries), "
                "(SELECT count(*) FROM content_versions), "
                "(SELECT count(*) FROM content_observations), "
                "(SELECT count(*) FROM content_visibility_observations)"
            )
        ).one()
        with pytest.raises(ApplicationError, match="idempotency_conflict"):
            ContentService(session).persist_post(owner_id=owner_id, command=command)

    assert completed.status is DeletionStatus.COMPLETED
    assert tuple(counts) == (0, 0, 0, 0, 0)
    assert content_client.get(f"/api/contents/{created.id}").status_code == 404


@pytest.mark.parametrize(
    ("extra_fields", "message"),
    [
        ({"text_scope": "summary", "text_origin": "source"}, "requires a title or body"),
        (
            {"text_scope": "truncated", "text_origin": "source", "body": "部分正文"},
            "requires truncation_reason",
        ),
        (
            {"text_scope": "media_only", "text_origin": "source", "body": "伪造媒体文本"},
            "cannot contain invented",
        ),
        (
            {"text_scope": "full", "text_origin": "machine_extracted", "body": "提取文本"},
            "requires text_origin_ref",
        ),
        (
            {
                "text_scope": "full",
                "text_origin": "source",
                "text_origin_ref": "media_extraction:1",
                "body": "来源原文",
            },
            "cannot declare a machine extraction reference",
        ),
        (
            {
                "text_scope": "full",
                "text_origin": "source",
                "body": "来源原文",
                "quote_target_author_external_id": "author-only",
            },
            "target identity requires an external_id",
        ),
        (
            {
                "published_at": "2026-09-22T07:00:00.1234567Z",
                "text_scope": "full",
                "text_origin": "source",
                "body": "来源原文",
            },
            "at most 6 fractional digits",
        ),
    ],
)
def test_content_version_contract_rejects_false_or_untraceable_text(
    content_client: TestClient,
    extra_fields: dict[str, object],
    message: str,
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, job_id, _ = _seed_context(content_client, owner_id)
    command = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=job_id,
        operation_id=uuid4(),
        observed_at=datetime.now(UTC) - timedelta(minutes=1),
        extra_fields=extra_fields,
    )

    with (
        content_client.app.state.session_factory() as session,
        pytest.raises(ValueError, match=message),
    ):
        ContentService(session).persist_post(owner_id=owner_id, command=command)


def test_content_versions_keep_truncated_media_and_machine_extracted_semantics(
    content_client: TestClient,
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, job_id, _ = _seed_context(content_client, owner_id)
    cases = (
        (
            "post-truncated",
            {
                "text_scope": "truncated",
                "text_origin": "source",
                "body": "来源只返回的前半段",
                "truncation_reason": "source_limit",
            },
        ),
        ("post-media", {"text_scope": "media_only", "text_origin": "source"}),
        (
            "post-extracted",
            {
                "text_scope": "full",
                "text_origin": "machine_extracted",
                "text_origin_ref": "media_extraction:controlled-001",
                "body": "受控提取文本",
            },
        ),
    )
    observed_at = datetime.now(UTC) - timedelta(minutes=1)
    created = []
    with content_client.app.state.session_factory() as session:
        service = ContentService(session)
        for index, (external_id, fields) in enumerate(cases):
            created.append(
                service.persist_post(
                    owner_id=owner_id,
                    command=_command(
                        owner_id=owner_id,
                        connection_id=connection_id,
                        policy_id=policy_id,
                        retention_id=retention_id,
                        job_id=job_id,
                        operation_id=uuid4(),
                        observed_at=observed_at + timedelta(seconds=index),
                        external_id=external_id,
                        extra_fields=fields,
                    ),
                )
            )

    versions = [item.latest_observation.content_version for item in created]
    assert [version.text_scope if version else None for version in versions] == [
        "truncated",
        "media_only",
        "full",
    ]
    assert versions[0] is not None
    assert versions[0].truncation_reason == "source_limit"
    assert versions[1] is not None
    assert versions[1].title is None and versions[1].body is None
    assert versions[2] is not None
    assert versions[2].text_origin == "machine_extracted"
    assert versions[2].text_origin_ref == "media_extraction:controlled-001"


def test_same_post_keeps_two_discoveries_zero_unknown_and_idempotent_observations(
    content_client: TestClient,
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, first_job_id, second_job_id = _seed_context(
        content_client, owner_id
    )
    observed_at = datetime.now(UTC) - timedelta(minutes=2)
    first_operation = uuid4()
    second_operation = uuid4()
    first = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=first_job_id,
        operation_id=first_operation,
        observed_at=observed_at,
    )
    second = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=second_job_id,
        operation_id=second_operation,
        observed_at=observed_at + timedelta(minutes=1),
    )
    factory = content_client.app.state.session_factory

    with factory() as session:
        service = ContentService(session)
        created = service.persist_post(owner_id=owner_id, command=first)
        replayed = service.persist_post(owner_id=owner_id, command=first)
        updated = service.persist_post(owner_id=owner_id, command=second)
        with pytest.raises(ApplicationError, match="idempotency_conflict"):
            service.persist_post(
                owner_id=owner_id,
                command=first.model_copy(
                    update={
                        "admission": first.admission.model_copy(
                            update={"fields": {**first.admission.fields, "like_count": 1}}
                        )
                    }
                ),
            )

    assert created.id == replayed.id == updated.id
    assert created.latest_observation.id == replayed.latest_observation.id
    listed = content_client.get("/api/contents")
    detail = content_client.get(f"/api/contents/{created.id}")

    assert listed.status_code == 200, listed.json()
    assert listed.headers["cache-control"] == "no-store"
    assert len(listed.json()["items"]) == 1
    item = listed.json()["items"][0]
    assert item["discovery_count"] == 2
    assert item["latest_observation"]["metrics"]["like_count"] == 0
    assert item["latest_observation"]["metrics"]["view_count"] is None
    assert detail.status_code == 200
    assert {entry["job_id"] for entry in detail.json()["discoveries"]} == {
        str(first_job_id),
        str(second_job_id),
    }
    with factory() as session:
        counts = session.execute(
            text(
                "SELECT (SELECT count(*) FROM content_records), "
                "(SELECT count(*) FROM content_discoveries), "
                "(SELECT count(*) FROM content_observations), "
                "(SELECT count(*) FROM evidence_resources), "
                "(SELECT count(*) FROM source_capability_evidence)"
            )
        ).one()
    assert tuple(counts) == (1, 2, 2, 2, 2)


@pytest.mark.parametrize("change", ["replace", "disable"])
def test_connection_changes_preserve_historical_content_and_replay(
    content_client: TestClient, change: str
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, job_id, _ = _seed_context(content_client, owner_id)
    command = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=job_id,
        operation_id=uuid4(),
        observed_at=datetime.now(UTC) - timedelta(minutes=1),
        extra_fields={"body": "historical body", "text_scope": "full", "text_origin": "source"},
    )
    factory = content_client.app.state.session_factory
    with factory() as session:
        original = ContentService(session).persist_post(owner_id=owner_id, command=command)
        with session.begin():
            if change == "replace":
                session.execute(
                    text(
                        "INSERT INTO source_connection_versions "
                        "(connection_id, version, owner_id, secret_ref, created_by, created_at) "
                        "SELECT connection_id, 2, owner_id, secret_ref, created_by, now() "
                        "FROM source_connection_versions WHERE connection_id = :id AND version = 1"
                    ),
                    {"id": connection_id},
                )
                session.execute(
                    text("UPDATE source_connections SET current_version = 2 WHERE id = :id"),
                    {"id": connection_id},
                )
            else:
                session.execute(
                    text("UPDATE source_connections SET status = 'disabled' WHERE id = :id"),
                    {"id": connection_id},
                )
        replayed = ContentService(session).persist_post(owner_id=owner_id, command=command)
        assert replayed == original
        assert session.execute(
            text("SELECT connection_version FROM source_capability_evidence")
        ).all() == [(1,)]
        assert session.scalar(text("SELECT count(*) FROM content_observations")) == 1
    response = content_client.get(f"/api/contents/{original.id}")
    assert response.status_code == 200
    assert "historical body" in response.text


def test_content_reads_enforce_owner_and_lifecycle_boundary(content_client: TestClient) -> None:
    assert content_client.get("/api/contents").status_code == 401
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, job_id, _ = _seed_context(content_client, owner_id)
    command = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=job_id,
        operation_id=uuid4(),
        observed_at=datetime.now(UTC) - timedelta(minutes=1),
    )
    factory = content_client.app.state.session_factory
    with factory() as session:
        created = ContentService(session).persist_post(owner_id=owner_id, command=command)
        with pytest.raises(ApplicationError, match="resource_not_found"):
            ContentService(session).get_content(owner_id=uuid4(), content_id=created.id)
        LifecycleService(session).request_deletion(
            owner_id=owner_id,
            operation_id=uuid4(),
            resource_type="content_observation",
            resource_id=created.latest_observation.id,
            reason=DeletionReason.USER_REQUEST,
        )

    hidden = content_client.get(f"/api/contents/{created.id}")
    listed = content_client.get("/api/contents")
    assert hidden.status_code == 404
    assert hidden.json()["code"] == "resource_not_found"
    assert listed.status_code == 200
    assert listed.json() == {"items": [], "next_cursor": None}


@pytest.mark.parametrize(
    ("reference", "code"),
    [
        ("job_id", "resource_not_found"),
        ("connection_id", "resource_not_found"),
        ("connection_version", "connection_version_conflict"),
        ("disabled_connection", "connection_disabled"),
    ],
)
def test_unavailable_relation_rolls_back_entire_content_write(
    content_client: TestClient, reference: str, code: str
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, job_id, _ = _seed_context(content_client, owner_id)
    command = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=job_id,
        operation_id=uuid4(),
        observed_at=datetime.now(UTC) - timedelta(minutes=1),
        extra_fields={"body": "private body", "text_scope": "full", "text_origin": "source"},
    )
    factory = content_client.app.state.session_factory
    with factory() as session:
        if reference == "disabled_connection":
            with session.begin():
                session.execute(
                    text("UPDATE source_connections SET status = 'disabled' WHERE id = :id"),
                    {"id": connection_id},
                )
            rejected = command
        else:
            value = 2 if reference == "connection_version" else uuid4()
            rejected = command.model_copy(update={reference: value})
        with pytest.raises(ApplicationError, match=code):
            ContentService(session).persist_post(owner_id=owner_id, command=rejected)
        for table in (
            "content_records",
            "content_versions",
            "content_observations",
            "content_discoveries",
            "content_visibility_observations",
            "evidence_resources",
            "source_capability_evidence",
        ):
            assert session.scalar(text(f"SELECT count(*) FROM {table}")) == 0
        if reference == "disabled_connection":
            session.execute(
                text("UPDATE source_connections SET status = 'active' WHERE id = :id"),
                {"id": connection_id},
            )
            session.commit()
        saved = ContentService(session).persist_post(owner_id=owner_id, command=command)
        outsider = uuid4()
        with pytest.raises(ApplicationError, match="resource_not_found"):
            ContentService(session).get_content(owner_id=outsider, content_id=saved.id)
        assert ContentService(session).list_contents(owner_id=outsider, cursor=None, limit=20) == (
            [],
            None,
        )
        platforms = SourceConnectionService(session).list_platforms(owner_id=outsider)
        for platform in platforms:
            assert platform.connection_version is None and not platform.has_credentials
            for capability in platform.capabilities:
                assert capability.manual.last_checked_at is None
                assert capability.manual.last_persisted_success_at is None
    response = content_client.get(f"/api/contents/{saved.id}")
    assert response.status_code == 200
    assert response.headers["cache-control"] == "no-store"


def test_reference_expansion_rechecks_target_readability(content_client: TestClient) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, job_id, _ = _seed_context(content_client, owner_id)
    base = _command(
        owner_id=owner_id,
        connection_id=connection_id,
        policy_id=policy_id,
        retention_id=retention_id,
        job_id=job_id,
        operation_id=uuid4(),
        observed_at=datetime.now(UTC) - timedelta(minutes=1),
        external_id="target",
        extra_fields={"body": "private target", "text_scope": "full", "text_origin": "source"},
    )
    subject = base.model_copy(
        update={
            "source_operation_id": uuid4(),
            "admission": base.admission.model_copy(
                update={
                    "fields": {
                        **base.admission.fields,
                        "external_id": "subject",
                        "body": "subject only",
                        "quote_target_external_id": "target",
                    }
                }
            ),
        }
    )
    with content_client.app.state.session_factory() as session:
        service = ContentService(session)
        target = service.persist_post(owner_id=owner_id, command=base)
        saved = service.persist_post(owner_id=owner_id, command=subject)
        assert saved.latest_observation.content_version.relations[0].target_content_id == target.id
        LifecycleService(session).request_deletion(
            owner_id=owner_id,
            operation_id=uuid4(),
            resource_type="content_observation",
            resource_id=target.latest_observation.id,
            reason=DeletionReason.USER_REQUEST,
        )
        current = service.get_content(owner_id=owner_id, content_id=saved.id)
        assert current.latest_observation.content_version.relations[0].target_content_id is None
        assert "private target" not in current.model_dump_json()


def test_concurrent_writes_reuse_the_same_native_content_identity(
    content_client: TestClient,
) -> None:
    owner_id = _initialize(content_client)
    connection_id, policy_id, retention_id, first_job_id, second_job_id = _seed_context(
        content_client, owner_id
    )
    observed_at = datetime.now(UTC) - timedelta(minutes=1)
    commands = (
        _command(
            owner_id=owner_id,
            connection_id=connection_id,
            policy_id=policy_id,
            retention_id=retention_id,
            job_id=first_job_id,
            operation_id=uuid4(),
            observed_at=observed_at,
        ),
        _command(
            owner_id=owner_id,
            connection_id=connection_id,
            policy_id=policy_id,
            retention_id=retention_id,
            job_id=second_job_id,
            operation_id=uuid4(),
            observed_at=observed_at + timedelta(seconds=1),
        ),
    )
    factory = content_client.app.state.session_factory

    def persist(command: PersistContentPostInput) -> UUID:
        with factory() as session:
            return (
                ContentService(session)
                .persist_post(
                    owner_id=owner_id,
                    command=command,
                )
                .id
            )

    with ThreadPoolExecutor(max_workers=2) as executor:
        content_ids = list(executor.map(persist, commands))

    assert content_ids[0] == content_ids[1]
    with factory() as session:
        counts = session.execute(
            text(
                "SELECT (SELECT count(*) FROM content_records), "
                "(SELECT count(*) FROM content_discoveries), "
                "(SELECT count(*) FROM content_observations)"
            )
        ).one()
    assert tuple(counts) == (1, 2, 2)


def _seed_comment_context(
    client: TestClient, owner_id: UUID, connection_id: UUID
) -> tuple[UUID, UUID, UUID]:
    policy_id = uuid4()
    retention_id = uuid4()
    job_id = uuid4()
    now = datetime.now(UTC)
    fields = {
        "object_type": "评论类型",
        "external_id": "评论身份",
        "post_external_id": "所属作品",
        "parent_comment_external_id": "父评论",
        "canonical_url": "评论入口",
        "author_external_id": "公开作者身份",
        "author_name": "公开作者昵称",
        "published_at": "发布时间",
        "like_count": "点赞观察",
        "text_scope": "正文完整度",
        "text_origin": "正文来源",
        "body": "评论正文",
    }
    factory = client.app.state.session_factory
    with factory() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO source_access_policies "
                "(id, owner_id, source_key, capability, status, enabled, access_basis, "
                "terms_reference, processing_purpose, component_name, component_version, "
                "component_license, field_purposes, reviewed_at, review_expires_at, "
                "policy_version, created_at, updated_at) VALUES "
                "(:id, :owner_id, 'x', 'comments', 'approved', true, 'official_api', "
                "'https://developer.x.com/terms', '受控评论资料验证', 'controlled-collector', "
                "'1', 'MIT', CAST(:fields AS jsonb), :now, NULL, 1, :now, :now)"
            ),
            {
                "id": policy_id,
                "owner_id": owner_id,
                "fields": json.dumps(fields, ensure_ascii=False),
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
        session.execute(
            text(
                "INSERT INTO jobs "
                "(id, owner_id, operation_id, kind, configuration_ref, "
                "configuration_version, source_key, source_capability, scope, "
                "request_fingerprint, status, created_at, updated_at) VALUES "
                "(:id, :owner_id, :operation_id, 'source.comments', 'topic:controlled-comments', "
                "1, 'x', 'comments', '{}'::jsonb, :fingerprint, 'queued', :now, :now)"
            ),
            {
                "id": job_id,
                "owner_id": owner_id,
                "operation_id": uuid4(),
                "fingerprint": bytes([9]) * 32,
                "now": now,
            },
        )
    del connection_id
    return policy_id, retention_id, job_id


def _comment_command(
    *,
    owner_id: UUID,
    connection_id: UUID,
    policy_id: UUID,
    retention_id: UUID,
    job_id: UUID,
    external_id: str,
    post_external_id: str | None,
    parent_comment_external_id: str | None = None,
    author_name: str | None = "楼主",
) -> PersistContentPostInput:
    observed_at = datetime.now(UTC) - timedelta(minutes=1)
    fields: dict[str, object] = {
        "object_type": "comment",
        "external_id": external_id,
        "author_external_id": "commenter-1",
        "author_name": author_name,
        "published_at": "2026-09-22T08:00:00Z",
        "like_count": 3,
        "text_scope": "full",
        "text_origin": "source",
        "body": f"评论 {external_id}",
    }
    if post_external_id is not None:
        fields["post_external_id"] = post_external_id
    if parent_comment_external_id is not None:
        fields["parent_comment_external_id"] = parent_comment_external_id
    return PersistContentPostInput(
        job_id=job_id,
        source_operation_id=uuid4(),
        connection_id=connection_id,
        connection_version=1,
        entry_point=SourceEntryPoint.MANUAL,
        component_name="controlled-collector",
        component_version="1",
        native_scope=None,
        admission=AdmittedSourcePayload(
            policy_id=policy_id,
            policy_version=1,
            owner_id=owner_id,
            source_key="x",
            capability=SourceCapability.COMMENTS,
            retention_policy_id=retention_id,
            retention_policy_version=1,
            data_class=DataClass.STRUCTURED,
            collected_at=observed_at,
            expires_at=observed_at + timedelta(days=30),
            fields=fields,
        ),
    )


def _thread_rows(client: TestClient, owner_id: UUID) -> dict[str, tuple[str, str | None]]:
    factory = client.app.state.session_factory
    with factory() as session:
        rows = session.execute(
            text(
                "SELECT c.external_id, p.external_id, pa.external_id "
                "FROM content_threads t "
                "JOIN content_records c ON c.owner_id = t.owner_id AND c.id = t.content_id "
                "JOIN content_records p ON p.owner_id = t.owner_id AND p.id = t.post_content_id "
                "LEFT JOIN content_records pa "
                "ON pa.owner_id = t.owner_id AND pa.id = t.parent_content_id "
                "WHERE t.owner_id = :owner_id"
            ),
            {"owner_id": owner_id},
        ).all()
    return {row[0]: (row[1], row[2]) for row in rows}


def test_comments_keep_post_and_parent_links_even_when_reply_arrives_first(
    content_client: TestClient,
) -> None:
    owner_id = _initialize(content_client)
    connection_id, *_ = _seed_context(content_client, owner_id)
    policy_id, retention_id, job_id = _seed_comment_context(content_client, owner_id, connection_id)
    factory = content_client.app.state.session_factory

    def persist(**kwargs: object) -> None:
        with factory() as session:
            ContentService(session).persist_comment(
                owner_id=owner_id,
                command=_comment_command(
                    owner_id=owner_id,
                    connection_id=connection_id,
                    policy_id=policy_id,
                    retention_id=retention_id,
                    job_id=job_id,
                    **kwargs,  # type: ignore[arg-type]
                ),
            )

    persist(external_id="reply-1", post_external_id="post-9", parent_comment_external_id="c-1")
    assert _thread_rows(content_client, owner_id) == {"reply-1": ("post-9", "c-1")}

    persist(external_id="c-1", post_external_id="post-9")
    persist(external_id="reply-1", post_external_id="post-9", parent_comment_external_id="c-1")
    assert _thread_rows(content_client, owner_id) == {
        "reply-1": ("post-9", "c-1"),
        "c-1": ("post-9", None),
    }

    with factory() as session:
        counts = dict(
            session.execute(
                text(
                    "SELECT object_type, count(*) FROM content_records "
                    "WHERE owner_id = :owner_id GROUP BY object_type"
                ),
                {"owner_id": owner_id},
            ).all()
        )
        author_names = set(
            session.scalars(
                text(
                    "SELECT author_name FROM content_observations "
                    "WHERE owner_id = :owner_id AND author_name IS NOT NULL"
                ),
                {"owner_id": owner_id},
            )
        )
    assert counts == {"post": 1, "comment": 2}
    assert author_names == {"楼主"}

    with pytest.raises(ValueError, match="thread"):
        persist(external_id="reply-1", post_external_id="post-other")
    with pytest.raises(ValueError, match="post_external_id"):
        persist(external_id="orphan", post_external_id=None)
