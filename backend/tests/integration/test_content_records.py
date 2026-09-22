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
from content.schemas import PersistContentPostInput
from content.services import ContentService
from core.config import Settings
from core.errors import ApplicationError
from evidence.schemas import AdmittedSourcePayload, DataClass, DeletionReason
from evidence.services import LifecycleService
from main import create_app
from sources.contracts import SourceCapability

_BOOTSTRAP_TOKEN = "content-records-isolated-bootstrap-token"
_PASSWORD = "correct horse battery staple"
_TRUNCATE = (
    "TRUNCATE content_discoveries, content_observations, content_records, "
    "source_capability_evidence, source_connection_versions, source_connections, "
    "provenance_manifest_inputs, provenance_manifests, evidence_cleanup_targets, "
    "evidence_deletions, evidence_resources, evidence_retention_policies, "
    "source_access_policies, resource_budget_reservations, resource_budget_windows, "
    "resource_budget_policies, resource_usage_attempts, resource_component_policies, "
    "job_stage_attempts, processed_messages, job_attempts, outbox_messages, jobs, "
    "monitor_topic_versions, monitor_topics, identity_sessions, identity_users"
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
) -> PersistContentPostInput:
    return PersistContentPostInput(
        job_id=job_id,
        source_operation_id=operation_id,
        connection_id=connection_id,
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
                "external_id": "post-001",
                "canonical_url": "https://example.invalid/posts/1",
                "author_external_id": "author-1",
                "published_at": "2026-09-22T07:00:00Z",
                "like_count": like_count,
                "comment_count": None,
                "repost_count": 0,
                "view_count": None,
                "play_count": None,
                "danmaku_count": None,
            },
        ),
    )


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
