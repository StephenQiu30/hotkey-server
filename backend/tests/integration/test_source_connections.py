from __future__ import annotations

import os
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text

from connections.services import SourceConnectionService
from core.config import Settings
from main import create_app

_BOOTSTRAP_TOKEN = "source-connection-bootstrap-token"
_PASSWORD = "correct horse battery staple"
_TRUNCATE = (
    "TRUNCATE source_capability_evidence, source_connection_versions, source_connections, "
    "provenance_manifest_inputs, provenance_manifests, evidence_cleanup_targets, "
    "evidence_deletions, evidence_resources, evidence_retention_policies, "
    "source_access_policies, resource_budget_reservations, resource_budget_windows, "
    "resource_budget_policies, resource_usage_attempts, resource_component_policies, "
    "job_stage_attempts, processed_messages, job_attempts, outbox_messages, jobs, "
    "monitor_topic_versions, monitor_topics, identity_sessions, identity_users"
)


@pytest.fixture
def source_connection_client() -> Iterator[TestClient]:
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


def _initialize(client: TestClient) -> str:
    response = client.post(
        "/api/identity/initialize",
        headers={
            "X-HotKey-Bootstrap-Token": _BOOTSTRAP_TOKEN,
            "X-HotKey-CSRF": "1",
        },
        json={"username": "owner", "password": _PASSWORD},
    )
    assert response.status_code == 201
    return response.json()["user"]["id"]


def _capability(platform: dict[str, object], name: str) -> dict[str, object]:
    capabilities = platform["capabilities"]
    assert isinstance(capabilities, list)
    return next(item for item in capabilities if item["capability"] == name)


def test_capability_catalog_requires_session_and_reports_truthful_defaults(
    source_connection_client: TestClient,
) -> None:
    missing = source_connection_client.get("/api/source-capabilities")
    _initialize(source_connection_client)

    response = source_connection_client.get("/api/source-capabilities")

    assert missing.status_code == 401
    assert response.status_code == 200
    assert response.headers["cache-control"] == "no-store"
    assert response.json()["next_cursor"] is None
    platforms = response.json()["items"]
    assert [platform["source_key"] for platform in platforms] == ["x", "douyin"]
    assert platforms[0]["status"] == "restricted"
    assert platforms[0]["rollout_role"] == "required"
    assert platforms[1]["status"] == "unconfigured"
    assert platforms[1]["rollout_role"] == "candidate"
    assert {item["capability"] for item in platforms[1]["capabilities"]} == {
        "search",
        "author_posts",
        "comments",
        "replies",
    }
    assert "secret" not in response.text.lower()
    assert "owner_id" not in response.text


def test_current_version_persisted_evidence_projects_partial_without_cross_entry_leak(
    source_connection_client: TestClient,
) -> None:
    owner_id = _initialize(source_connection_client)
    factory = source_connection_client.app.state.session_factory
    connection_id = uuid4()
    policy_id = uuid4()
    now = datetime.now(UTC)
    with factory() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO source_connections "
                "(id, owner_id, source_key, status, current_version, created_at, updated_at) "
                "VALUES (:id, :owner_id, 'douyin', 'active', 1, :now, :now)"
            ),
            {"id": connection_id, "owner_id": owner_id, "now": now},
        )
        session.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, secret_ref, created_by, created_at) "
                "VALUES (:connection_id, 1, :owner_id, :secret_ref, :owner_id, :now)"
            ),
            {
                "connection_id": connection_id,
                "owner_id": owner_id,
                "secret_ref": "env:HOTKEY_DOUYIN_TOKEN",
                "now": now,
            },
        )
        session.execute(
            text(
                "INSERT INTO source_access_policies "
                "(id, owner_id, source_key, capability, status, enabled, access_basis, "
                "terms_reference, processing_purpose, component_name, component_version, "
                "component_license, field_purposes, reviewed_at, review_expires_at, "
                "policy_version, created_at, updated_at) VALUES "
                "(:id, :owner_id, 'douyin', 'search', 'approved', true, 'official_api', "
                ":terms, '热点检索', 'douyin-open-api', 'documented', 'platform-terms', "
                "CAST(:fields AS jsonb), :reviewed_at, :expires_at, 1, :now, :now)"
            ),
            {
                "id": policy_id,
                "owner_id": owner_id,
                "terms": "https://open.douyin.com/",
                "fields": '{"external_id":"持久化检索结果"}',
                "reviewed_at": now - timedelta(days=1),
                "expires_at": now + timedelta(days=30),
                "now": now,
            },
        )
        for entry_point, kind, resource_ref in (
            ("manual", "persisted_read", "evidence:controlled-search-page"),
            ("scheduled", "probe", None),
        ):
            session.execute(
                text(
                    "INSERT INTO source_capability_evidence "
                    "(id, operation_id, owner_id, connection_id, connection_version, "
                    "capability, entry_point, kind, outcome, stop_reason, resource_ref, "
                    "component_name, component_version, observed_at, created_at) VALUES "
                    "(:id, :operation_id, :owner_id, :connection_id, 1, 'search', "
                    ":entry_point, :kind, 'succeeded', NULL, :resource_ref, "
                    "'controlled-fixture', '1', :now, :now)"
                ),
                {
                    "id": uuid4(),
                    "operation_id": uuid4(),
                    "owner_id": owner_id,
                    "connection_id": connection_id,
                    "entry_point": entry_point,
                    "kind": kind,
                    "resource_ref": resource_ref,
                    "now": now,
                },
            )

    response = source_connection_client.get("/api/source-capabilities")

    assert response.status_code == 200
    douyin = next(item for item in response.json()["items"] if item["source_key"] == "douyin")
    search = _capability(douyin, "search")
    assert douyin["status"] == "partial"
    assert douyin["connection_version"] == 1
    assert douyin["has_credentials"] is True
    assert search["manual"]["status"] == "available"
    assert search["manual"]["last_persisted_success_at"] is not None
    assert search["scheduled"]["status"] == "pending_verification"
    assert search["scheduled"]["last_persisted_success_at"] is None
    assert "HOTKEY_DOUYIN_TOKEN" not in response.text

    with factory() as session:
        other_owner = SourceConnectionService(session).list_platforms(owner_id=uuid4())
    other_douyin = next(item for item in other_owner if item.source_key == "douyin")
    assert other_douyin.status == "unconfigured"
    assert other_douyin.connection_version is None

    with factory() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, secret_ref, created_by, created_at) "
                "VALUES (:connection_id, 2, :owner_id, :secret_ref, :owner_id, :now)"
            ),
            {
                "connection_id": connection_id,
                "owner_id": owner_id,
                "secret_ref": "env:HOTKEY_DOUYIN_TOKEN_V2",
                "now": now + timedelta(minutes=1),
            },
        )
        session.execute(
            text(
                "UPDATE source_connections SET current_version = 2, updated_at = :now "
                "WHERE id = :connection_id"
            ),
            {"connection_id": connection_id, "now": now + timedelta(minutes=1)},
        )

    refreshed = source_connection_client.get("/api/source-capabilities")
    douyin = next(item for item in refreshed.json()["items"] if item["source_key"] == "douyin")
    search = _capability(douyin, "search")
    assert douyin["connection_version"] == 2
    assert search["manual"]["status"] == "pending_verification"
    assert search["manual"]["last_persisted_success_at"] is None
