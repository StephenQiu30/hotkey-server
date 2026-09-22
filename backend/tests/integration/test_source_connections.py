from __future__ import annotations

import os
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text
from typer.testing import CliRunner

from cli.commands import app as cli_app
from connections.schemas import (
    ConnectionEvidenceOutcome,
    PersistedReadEvidenceInput,
    SourceEntryPoint,
)
from connections.services import SourceCapabilityEvidenceService, SourceConnectionService
from core.config import Settings, get_settings
from core.errors import ApplicationError
from main import create_app
from sources.contracts import SourceCapability

_BOOTSTRAP_TOKEN = "source-connection-bootstrap-token"
_PASSWORD = "correct horse battery staple"
_TRUNCATE = (
    "TRUNCATE content_version_relations, content_visibility_observations, "
    "content_observations, content_versions, "
    "content_discoveries, content_records, "
    "source_capability_evidence, source_connection_versions, source_connections, "
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


def _seed_search_connection(client: TestClient, owner_id: str) -> UUID:
    factory = client.app.state.session_factory
    connection_id = uuid4()
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
                "id": uuid4(),
                "owner_id": owner_id,
                "terms": "https://open.douyin.com/",
                "fields": '{"external_id":"持久化检索结果"}',
                "reviewed_at": now - timedelta(days=1),
                "expires_at": now + timedelta(days=30),
                "now": now,
            },
        )
    return connection_id


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
    connection_id = _seed_search_connection(source_connection_client, owner_id)
    now = datetime.now(UTC)
    with factory() as session, session.begin():
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


def test_probe_cli_records_check_without_enabling_either_entry(
    source_connection_client: TestClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    owner_id = _initialize(source_connection_client)
    connection_id = _seed_search_connection(source_connection_client, owner_id)
    operation_id = uuid4()
    database_url = os.environ["HOTKEY_TEST_DATABASE_URL"]
    monkeypatch.setenv("HOTKEY_DATABASE_URL", database_url)
    monkeypatch.setenv("HOTKEY_ENVIRONMENT", "test")
    get_settings.cache_clear()
    try:
        result = CliRunner().invoke(
            cli_app,
            [
                "connections",
                "record-probe",
                "--owner-id",
                owner_id,
                "--connection-id",
                str(connection_id),
                "--operation-id",
                str(operation_id),
                "--capability",
                "search",
                "--entry-point",
                "manual",
                "--outcome",
                "succeeded",
                "--component-name",
                "controlled-probe",
                "--component-version",
                "1",
            ],
        )
    finally:
        get_settings.cache_clear()

    assert result.exit_code == 0, result.output
    assert "Probe evidence recorded" in result.stdout
    assert "HOTKEY_DOUYIN_TOKEN" not in result.stdout
    factory = source_connection_client.app.state.session_factory
    with factory() as session:
        stored = session.execute(
            text(
                "SELECT kind, outcome, resource_ref FROM source_capability_evidence "
                "WHERE owner_id = :owner_id AND operation_id = :operation_id"
            ),
            {"owner_id": owner_id, "operation_id": operation_id},
        ).one()
    assert tuple(stored) == ("probe", "succeeded", None)

    response = source_connection_client.get("/api/source-capabilities")
    douyin = next(item for item in response.json()["items"] if item["source_key"] == "douyin")
    search = _capability(douyin, "search")
    assert search["manual"]["status"] == "pending_verification"
    assert search["manual"]["last_checked_at"] is not None
    assert search["manual"]["last_persisted_success_at"] is None
    assert search["scheduled"]["status"] == "pending_verification"
    assert search["scheduled"]["last_checked_at"] is None


def test_persisted_read_registration_is_idempotent_and_entry_scoped(
    source_connection_client: TestClient,
) -> None:
    owner_id = UUID(_initialize(source_connection_client))
    connection_id = _seed_search_connection(source_connection_client, str(owner_id))
    operation_id = uuid4()
    command = PersistedReadEvidenceInput(
        operation_id=operation_id,
        connection_id=connection_id,
        capability=SourceCapability.SEARCH,
        entry_point=SourceEntryPoint.MANUAL,
        outcome=ConnectionEvidenceOutcome.SUCCEEDED,
        stop_reason=None,
        resource_ref="content:controlled-search-page",
        component_name="controlled-collector",
        component_version="1",
    )
    factory = source_connection_client.app.state.session_factory

    with factory() as session:
        service = SourceCapabilityEvidenceService(session)
        created = service.record_persisted_read(owner_id=owner_id, command=command)
        replayed = service.record_persisted_read(owner_id=owner_id, command=command)
        with pytest.raises(ApplicationError, match="idempotency_conflict") as captured:
            service.record_persisted_read(
                owner_id=owner_id,
                command=command.model_copy(
                    update={"resource_ref": "content:different-search-page"}
                ),
            )

    assert created.id == replayed.id
    assert created.kind == "persisted_read"
    assert created.connection_version == 1
    assert captured.value.code == "idempotency_conflict"
    with factory() as session:
        count = session.execute(
            text(
                "SELECT count(*) FROM source_capability_evidence "
                "WHERE owner_id = :owner_id AND operation_id = :operation_id"
            ),
            {"owner_id": owner_id, "operation_id": operation_id},
        ).scalar_one()
    assert count == 1

    response = source_connection_client.get("/api/source-capabilities")
    douyin = next(item for item in response.json()["items"] if item["source_key"] == "douyin")
    search = _capability(douyin, "search")
    assert search["manual"]["status"] == "available"
    assert search["manual"]["last_persisted_success_at"] is not None
    assert search["scheduled"]["status"] == "pending_verification"
    assert search["scheduled"]["last_persisted_success_at"] is None


def test_evidence_registration_rejects_another_owners_connection(
    source_connection_client: TestClient,
) -> None:
    owner_id = UUID(_initialize(source_connection_client))
    connection_id = _seed_search_connection(source_connection_client, str(owner_id))
    command = PersistedReadEvidenceInput(
        operation_id=uuid4(),
        connection_id=connection_id,
        capability=SourceCapability.SEARCH,
        entry_point=SourceEntryPoint.MANUAL,
        outcome=ConnectionEvidenceOutcome.SUCCEEDED,
        stop_reason=None,
        resource_ref="content:controlled-search-page",
        component_name="controlled-collector",
        component_version="1",
    )
    factory = source_connection_client.app.state.session_factory

    with factory() as session, pytest.raises(ApplicationError, match="resource_not_found"):
        SourceCapabilityEvidenceService(session).record_persisted_read(
            owner_id=uuid4(),
            command=command,
        )
