from __future__ import annotations

import os
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime, timedelta
from pathlib import Path
from threading import Event
from uuid import UUID, uuid4

import pytest
from fastapi.testclient import TestClient
from pydantic import SecretStr
from sqlalchemy import create_engine, text
from sqlalchemy.exc import IntegrityError
from typer.testing import CliRunner

from cli.commands import app as cli_app
from connections.adapters.local_secrets import BrowserStateError, BrowserStateStore
from connections.schemas import (
    ConnectionEvidenceOutcome,
    PersistedReadEvidenceInput,
    ProbeEvidenceInput,
    SourceCapabilityEvidenceView,
    SourceConnectionStatus,
    SourceConnectionUpdateInput,
    SourceEntryPoint,
)
from connections.services import (
    SourceCapabilityEvidenceService,
    SourceConnectionService,
    require_browser_state_execution,
    require_web_connection_execution,
    source_credential_reference,
)
from core.config import Settings, get_settings
from core.errors import ApplicationError
from main import create_app
from sources.contracts import SourceCapability, SourceStopReason

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
    credential = SecretStr("controlled-source-credential-for-tests-only")
    client.app.state.settings.source_credentials = {"douyin": credential}
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
                "secret_ref": source_credential_reference("douyin", credential),
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
    assert [platform["source_key"] for platform in platforms] == ["x", "douyin", "web"]
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
    assert platforms[2]["status"] == "unconfigured"
    assert platforms[2]["rollout_role"] == "required"
    assert platforms[2]["credential_configured"] is False
    assert platforms[2]["has_credentials"] is False
    assert [item["capability"] for item in platforms[2]["capabilities"]] == ["page_content"]
    assert "secret" not in response.text.lower()
    assert "owner_id" not in response.text


def test_anonymous_web_connection_versions_without_fabricating_credentials(
    source_connection_client: TestClient,
) -> None:
    client = source_connection_client
    owner_id = _initialize(client)
    headers = {"X-HotKey-CSRF": client.cookies["hotkey_csrf"]}

    missing_scope = client.put(
        "/api/source-connections/web",
        headers=headers,
        json={"expected_version": 0, "status": "active"},
    )
    assert missing_scope.status_code == 422
    assert missing_scope.json()["code"] == "invalid_connection_configuration"

    created = client.put(
        "/api/source-connections/web",
        headers=headers,
        json={
            "expected_version": 0,
            "status": "active",
            "allowed_hosts": ["Example.COM."],
        },
    )

    assert created.status_code == 200
    assert created.json()["source_key"] == "web"
    assert created.json()["version"] == 1
    assert created.json()["allowed_hosts"] == ["example.com"]
    web = next(
        item
        for item in client.get("/api/source-capabilities").json()["items"]
        if item["source_key"] == "web"
    )
    assert web["status"] == "restricted"
    assert web["connection_status"] == "active"
    assert web["has_credentials"] is False
    assert web["credential_configured"] is False
    assert web["credential_update_available"] is False
    assert web["allowed_hosts"] == ["example.com"]

    with client.app.state.session_factory() as session, session.begin():
        execution = require_web_connection_execution(
            session,
            owner_id=UUID(owner_id),
            connection_id=UUID(created.json()["id"]),
            connection_version=1,
            target_url="HTTPS://Example.COM:443/path?keep=value#fragment",
        )
        assert execution.normalized_url == "https://example.com/path?keep=value"
        assert execution.allowed_hosts == frozenset({"example.com"})
        with pytest.raises(ApplicationError, match="source_target_not_allowed"):
            require_web_connection_execution(
                session,
                owner_id=UUID(owner_id),
                connection_id=UUID(created.json()["id"]),
                connection_version=1,
                target_url="https://other.example/path",
            )

    disabled = client.put(
        "/api/source-connections/web",
        headers=headers,
        json={"expected_version": 1, "status": "disabled"},
    )
    assert disabled.status_code == 200
    assert disabled.json()["version"] == 2
    with client.app.state.session_factory() as session:
        versions = session.execute(
            text(
                "SELECT version, auth_kind, secret_ref, configuration "
                "FROM source_connection_versions "
                "WHERE owner_id = :owner_id ORDER BY version"
            ),
            {"owner_id": owner_id},
        ).all()
    assert versions == [
        (1, "none", None, {"allowed_hosts": ["example.com"]}),
        (2, "none", None, {"allowed_hosts": ["example.com"]}),
    ]
    assert "secret" not in created.text.lower()
    assert "secret" not in disabled.text.lower()

    rejected_social_scope = client.put(
        "/api/source-connections/douyin",
        headers=headers,
        json={
            "expected_version": 0,
            "status": "active",
            "allowed_hosts": ["example.com"],
        },
    )
    assert rejected_social_scope.status_code == 422
    assert rejected_social_scope.json()["code"] == "invalid_connection_configuration"


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
                "secret_ref": source_credential_reference(
                    "douyin",
                    source_connection_client.app.state.settings.source_credentials["douyin"],
                ),
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
                "--connection-version",
                "1",
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
        connection_version=1,
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
        connection_version=1,
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


@pytest.mark.parametrize("change", ["replace", "disable"])
@pytest.mark.parametrize("kind", ["probe", "persisted_read"])
def test_changed_connection_rejects_late_evidence_but_preserves_replays(
    source_connection_client: TestClient, change: str, kind: str
) -> None:
    owner_id = UUID(_initialize(source_connection_client))
    connection_id = _seed_search_connection(source_connection_client, str(owner_id))
    factory = source_connection_client.app.state.session_factory
    command = PersistedReadEvidenceInput(
        operation_id=uuid4(),
        connection_id=connection_id,
        connection_version=1,
        capability=SourceCapability.SEARCH,
        entry_point=SourceEntryPoint.MANUAL,
        outcome=ConnectionEvidenceOutcome.SUCCEEDED,
        stop_reason=None,
        resource_ref="content:controlled-search-page",
        component_name="controlled-collector",
        component_version="1",
    )

    def record(
        service: SourceCapabilityEvidenceService, operation_id: UUID
    ) -> SourceCapabilityEvidenceView:
        updated = command.model_copy(update={"operation_id": operation_id})
        if kind == "probe":
            return service.record_probe(
                owner_id=owner_id,
                command=ProbeEvidenceInput.model_validate(
                    updated.model_dump(exclude={"resource_ref"})
                ),
            )
        return service.record_persisted_read(owner_id=owner_id, command=updated)

    with factory() as session:
        original = record(SourceCapabilityEvidenceService(session), command.operation_id)
    with factory() as session, session.begin():
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

    with factory() as session:
        service = SourceCapabilityEvidenceService(session)
        code = "connection_version_conflict" if change == "replace" else "connection_disabled"
        with pytest.raises(ApplicationError, match=code):
            record(service, uuid4())
        replayed = record(service, command.operation_id)
        assert replayed == original
        with pytest.raises(ApplicationError, match="idempotency_conflict"):
            service.record_persisted_read(
                owner_id=owner_id,
                command=command.model_copy(update={"resource_ref": "content:changed"}),
            )
        rows = session.execute(
            text(
                "SELECT connection_version FROM source_capability_evidence "
                "WHERE connection_id = :id"
            ),
            {"id": connection_id},
        ).all()
        assert rows == [(1,)]
    platform = source_connection_client.get("/api/source-capabilities").json()["items"][1]
    assert _capability(platform, "search")["manual"]["status"] != "available"


def test_evidence_waits_for_connection_lock_and_observes_committed_disable(
    source_connection_client: TestClient,
) -> None:
    owner_id = UUID(_initialize(source_connection_client))
    connection_id = _seed_search_connection(source_connection_client, str(owner_id))
    factory = source_connection_client.app.state.session_factory
    command = ProbeEvidenceInput(
        operation_id=uuid4(),
        connection_id=connection_id,
        connection_version=1,
        capability=SourceCapability.SEARCH,
        entry_point=SourceEntryPoint.MANUAL,
        outcome=ConnectionEvidenceOutcome.SUCCEEDED,
        stop_reason=None,
        component_name="controlled-probe",
        component_version="1",
    )
    started = Event()

    def record() -> SourceCapabilityEvidenceView:
        with factory() as session:
            session.execute(text("SET statement_timeout = '5s'"))
            session.commit()
            started.set()
            return SourceCapabilityEvidenceService(session).record_probe(
                owner_id=owner_id, command=command
            )

    with ThreadPoolExecutor(max_workers=1) as executor:
        with factory() as session, session.begin():
            session.execute(
                text("UPDATE source_connections SET status = 'disabled' WHERE id = :id"),
                {"id": connection_id},
            )
            future = executor.submit(record)
            assert started.wait(timeout=5)
            with pytest.raises(TimeoutError):
                future.result(timeout=0.1)
        with pytest.raises(ApplicationError, match="connection_disabled"):
            future.result(timeout=5)
    with factory() as session:
        assert session.scalar(text("SELECT count(*) FROM source_capability_evidence")) == 0


def test_connection_management_requires_auth_csrf_and_server_credentials(
    source_connection_client: TestClient,
) -> None:
    client = source_connection_client
    payload = {"expected_version": 0, "status": "active"}
    assert client.put("/api/source-connections/douyin", json=payload).status_code == 401
    _initialize(client)
    assert client.put("/api/source-connections/douyin", json=payload).status_code == 403
    headers = {"X-HotKey-CSRF": client.cookies["hotkey_csrf"]}
    missing = client.put("/api/source-connections/douyin", json=payload, headers=headers)
    assert missing.status_code == 409
    assert missing.json()["code"] == "connection_credentials_missing"
    assert missing.headers["cache-control"] == "no-store"


def test_connection_rotation_disable_and_resume_are_versioned_without_secrets(
    source_connection_client: TestClient,
) -> None:
    client = source_connection_client
    owner_id = _initialize(client)
    headers = {"X-HotKey-CSRF": client.cookies["hotkey_csrf"]}
    secret = "controlled-source-credential-for-tests-only"
    client.app.state.settings.source_credentials = {"douyin": SecretStr(secret)}
    payload = {"expected_version": 0, "status": "active"}
    created = client.put("/api/source-connections/douyin", headers=headers, json=payload)
    assert created.status_code == 200
    assert created.json()["version"] == 1
    repeated = client.put("/api/source-connections/douyin", headers=headers, json=payload)
    assert repeated.json() == created.json()
    client.app.state.settings.source_credentials = {"douyin": SecretStr(secret + "-rotated")}
    platform = client.get("/api/source-capabilities").json()["items"][1]
    assert platform["credential_update_available"]
    assert platform["status"] == "authentication_required"
    rotated = client.put(
        "/api/source-connections/douyin",
        headers=headers,
        json={"expected_version": 1, "status": "active"},
    )
    assert rotated.status_code == 200
    assert rotated.json()["version"] == 2
    platform = client.get("/api/source-capabilities").json()["items"][1]
    assert not platform["credential_update_available"]
    assert all(item["manual"]["status"] != "available" for item in platform["capabilities"])
    disabled = client.put(
        "/api/source-connections/douyin",
        headers=headers,
        json={"expected_version": 2, "status": "disabled"},
    )
    assert disabled.json()["version"] == 3
    stale = client.put(
        "/api/source-connections/douyin",
        headers=headers,
        json={"expected_version": 2, "status": "active"},
    )
    assert stale.status_code == 409
    resumed = client.put(
        "/api/source-connections/douyin",
        headers=headers,
        json={"expected_version": 3, "status": "active"},
    )
    assert resumed.json()["version"] == 4
    for response in (created, repeated, rotated, disabled, stale, resumed):
        assert secret not in response.text
        assert "settings:" not in response.text
        assert "secret_ref" not in response.text
        assert response.headers["cache-control"] == "no-store"
    with client.app.state.session_factory() as session:
        rows = session.execute(
            text(
                "SELECT version, created_by FROM source_connection_versions "
                "WHERE owner_id = :owner_id ORDER BY version"
            ),
            {"owner_id": owner_id},
        ).all()
    assert rows == [(version, UUID(owner_id)) for version in range(1, 5)]


def test_disabled_connection_rejects_new_jobs_without_outbox(
    source_connection_client: TestClient,
) -> None:
    client = source_connection_client
    owner_id = _initialize(client)
    _seed_search_connection(client, owner_id)
    headers = {"X-HotKey-CSRF": client.cookies["hotkey_csrf"]}
    disabled = client.put(
        "/api/source-connections/douyin",
        headers=headers,
        json={"expected_version": 1, "status": "disabled"},
    )
    assert disabled.status_code == 200
    response = client.post(
        "/api/jobs",
        headers=headers,
        json={
            "operation_id": str(uuid4()),
            "kind": "monitor.collect",
            "observation": {
                "configuration_ref": "monitor:test",
                "configuration_version": 1,
                "source_key": "douyin",
                "source_capability": "search",
            },
            "scope": {},
        },
    )
    assert response.status_code == 409
    assert response.json()["code"] == "connection_disabled"
    with client.app.state.session_factory() as session:
        assert session.scalar(text("SELECT count(*) FROM jobs")) == 0
        assert session.scalar(text("SELECT count(*) FROM outbox_messages")) == 0


@pytest.mark.parametrize(
    "reason", [SourceStopReason.ACCESS_DENIED, SourceStopReason.AUTHENTICATION_REQUIRED]
)
def test_connection_auth_failure_propagates_but_capability_denial_is_local(
    source_connection_client: TestClient,
    reason: SourceStopReason,
) -> None:
    client = source_connection_client
    owner_id = UUID(_initialize(client))
    connection_id = _seed_search_connection(client, str(owner_id))
    with client.app.state.session_factory() as session:
        service = SourceCapabilityEvidenceService(session)
        service.record_persisted_read(
            owner_id=owner_id,
            command=PersistedReadEvidenceInput(
                operation_id=uuid4(),
                connection_id=connection_id,
                connection_version=1,
                capability=SourceCapability.SEARCH,
                entry_point=SourceEntryPoint.MANUAL,
                outcome=ConnectionEvidenceOutcome.SUCCEEDED,
                stop_reason=None,
                resource_ref="content:fixture",
                component_name="fixture",
                component_version="1",
            ),
        )
        service.record_probe(
            owner_id=owner_id,
            command=ProbeEvidenceInput(
                operation_id=uuid4(),
                connection_id=connection_id,
                connection_version=1,
                capability=SourceCapability.COMMENTS,
                entry_point=SourceEntryPoint.MANUAL,
                outcome=ConnectionEvidenceOutcome.FAILED,
                stop_reason=reason,
                component_name="fixture",
                component_version="1",
            ),
        )
    platform = client.get("/api/source-capabilities").json()["items"][1]
    search_status = _capability(platform, "search")["manual"]["status"]
    accepted = client.post(
        "/api/jobs",
        headers={"X-HotKey-CSRF": client.cookies["hotkey_csrf"]},
        json={
            "operation_id": str(uuid4()),
            "kind": "monitor.collect",
            "observation": {
                "configuration_ref": "monitor:test",
                "configuration_version": 1,
                "source_key": "douyin",
                "source_capability": "search",
            },
            "scope": {},
        },
    )
    if reason is SourceStopReason.AUTHENTICATION_REQUIRED:
        assert accepted.status_code == 409
        assert accepted.json()["code"] == "connection_authentication_required"
        with client.app.state.session_factory() as session:
            assert session.scalar(text("SELECT count(*) FROM jobs")) == 0
            assert session.scalar(text("SELECT count(*) FROM outbox_messages")) == 0
        assert search_status == "authentication_required"
        assert all(
            item["scheduled"]["status"] == "authentication_required"
            for item in platform["capabilities"]
        )
    else:
        assert accepted.status_code == 202
        assert search_status == "available"
        assert _capability(platform, "comments")["manual"]["status"] == "restricted"


def test_concurrent_connection_requests_create_one_version_per_transition(
    source_connection_client: TestClient,
) -> None:
    client = source_connection_client
    owner_id = UUID(_initialize(client))
    credentials = {"douyin": SecretStr("controlled-source-credential-for-tests-only")}

    def update(expected_version: int, status: SourceConnectionStatus):
        with client.app.state.session_factory() as session:
            return SourceConnectionService(session, credentials=credentials).update_connection(
                owner_id=owner_id,
                source_key="douyin",
                command=SourceConnectionUpdateInput(
                    expected_version=expected_version, status=status
                ),
            )

    for expected, status in [
        (0, SourceConnectionStatus.ACTIVE),
        (1, SourceConnectionStatus.DISABLED),
    ]:
        with ThreadPoolExecutor(max_workers=2) as executor:
            futures = [executor.submit(update, expected, status) for _ in range(2)]
            first, second = [future.result(timeout=5) for future in futures]
        assert first == second
        assert first.version == expected + 1
    with client.app.state.session_factory() as session:
        assert session.scalar(text("SELECT count(*) FROM source_connections")) == 1
        assert session.scalar(text("SELECT count(*) FROM source_connection_versions")) == 2
        with pytest.raises(ApplicationError, match="resource_not_found"):
            SourceConnectionService(session, credentials=credentials).update_connection(
                owner_id=uuid4(),
                source_key="douyin",
                command=SourceConnectionUpdateInput(
                    expected_version=2, status=SourceConnectionStatus.DISABLED
                ),
            )


def test_disabled_connection_rejects_manual_retry_but_preserves_accepted_replay(
    source_connection_client: TestClient,
) -> None:
    client = source_connection_client
    owner_id = _initialize(client)
    _seed_search_connection(client, owner_id)
    headers = {"X-HotKey-CSRF": client.cookies["hotkey_csrf"]}
    payload = {
        "operation_id": str(uuid4()),
        "kind": "monitor.collect",
        "scope": {},
        "observation": {
            "configuration_ref": "monitor:test",
            "configuration_version": 1,
            "source_key": "douyin",
            "source_capability": "search",
        },
    }
    created = client.post("/api/jobs", headers=headers, json=payload)
    assert created.status_code == 202
    job_id = created.json()["job_id"]
    client.app.state.settings.source_credentials = {}
    disabled = client.put(
        "/api/source-connections/douyin",
        headers=headers,
        json={"expected_version": 1, "status": "disabled"},
    )
    assert disabled.status_code == 200
    assert client.post("/api/jobs", headers=headers, json=payload).json() == created.json()
    with client.app.state.session_factory() as session, session.begin():
        session.execute(
            text(
                "UPDATE jobs SET status = 'failed', manual_retry_allowed = true, "
                "completed_at = now() WHERE id = :id"
            ),
            {"id": job_id},
        )
    retry = client.post(f"/api/jobs/{job_id}/retry", headers=headers)
    assert retry.status_code == 409
    assert retry.json()["code"] == "connection_disabled"
    with client.app.state.session_factory() as session:
        assert session.scalar(text("SELECT count(*) FROM outbox_messages")) == 1
        assert (
            session.scalar(text("SELECT status FROM jobs WHERE id = :id"), {"id": job_id})
            == "failed"
        )


def test_browser_state_execution_requires_current_active_authenticated_version(
    source_connection_client: TestClient, tmp_path: Path
) -> None:
    client = source_connection_client
    owner_id = UUID(_initialize(client))
    connection_id = uuid4()
    root = tmp_path / "browser-states"
    root.mkdir(mode=0o700)
    store = BrowserStateStore(root)
    state = {"cookies": [], "origins": []}
    reference = store.save(owner_id=owner_id, connection_id=connection_id, version=1, state=state)
    now = datetime.now(UTC)
    factory = client.app.state.session_factory
    with factory() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO source_connections "
                "(id, owner_id, source_key, status, current_version, created_at, updated_at) "
                "VALUES (:id, :owner_id, 'browser_fixture', 'active', 1, :now, :now)"
            ),
            {"id": connection_id, "owner_id": owner_id, "now": now},
        )
        session.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, auth_kind, secret_ref, created_by, "
                "created_at) VALUES (:id, 1, :owner_id, 'browser_state', :reference, "
                ":owner_id, :now)"
            ),
            {"id": connection_id, "owner_id": owner_id, "reference": reference, "now": now},
        )

    def read(version: int, *, owner: UUID = owner_id) -> dict[str, object]:
        with factory() as session, session.begin():
            return require_browser_state_execution(
                session,
                owner_id=owner,
                connection_id=connection_id,
                connection_version=version,
                store=store,
            )

    assert read(1) == state
    with pytest.raises(ApplicationError, match="resource_not_found"):
        read(1, owner=uuid4())

    second_reference = store.save(
        owner_id=owner_id, connection_id=connection_id, version=2, state=state
    )
    with factory() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, auth_kind, secret_ref, created_by, "
                "created_at) VALUES (:id, 2, :owner_id, 'browser_state', :reference, "
                ":owner_id, :now)"
            ),
            {
                "id": connection_id,
                "owner_id": owner_id,
                "reference": second_reference,
                "now": now,
            },
        )
        session.execute(
            text("UPDATE source_connections SET current_version = 2 WHERE id = :id"),
            {"id": connection_id},
        )
    with pytest.raises(ApplicationError, match="connection_version_conflict"):
        read(1)
    assert read(2) == state

    with factory() as session:
        SourceCapabilityEvidenceService(session).record_probe(
            owner_id=owner_id,
            command=ProbeEvidenceInput(
                operation_id=uuid4(),
                connection_id=connection_id,
                connection_version=2,
                capability=SourceCapability.COMMENTS,
                entry_point=SourceEntryPoint.MANUAL,
                outcome=ConnectionEvidenceOutcome.FAILED,
                stop_reason=SourceStopReason.AUTHENTICATION_REQUIRED,
                component_name="browser-fixture",
                component_version="1",
            ),
        )
    with pytest.raises(ApplicationError, match="connection_authentication_required"):
        read(2)

    with factory() as session, session.begin():
        session.execute(
            text("UPDATE source_connections SET status = 'disabled' WHERE id = :id"),
            {"id": connection_id},
        )
    with pytest.raises(ApplicationError, match="connection_disabled"):
        read(2)

    missing_reference = f"browser-state:{owner_id.hex}/{connection_id.hex}/3"
    with factory() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, auth_kind, secret_ref, created_by, "
                "created_at) VALUES (:id, 3, :owner_id, 'browser_state', :reference, "
                ":owner_id, :now)"
            ),
            {
                "id": connection_id,
                "owner_id": owner_id,
                "reference": missing_reference,
                "now": now,
            },
        )
        session.execute(
            text(
                "UPDATE source_connections SET status = 'active', current_version = 3 "
                "WHERE id = :id"
            ),
            {"id": connection_id},
        )
    with pytest.raises(ApplicationError, match="connection_credentials_missing"):
        read(3)

    for invalid_reference in (None, f"browser-state:{uuid4().hex}/{connection_id.hex}/4"):
        with pytest.raises(IntegrityError), factory() as session, session.begin():
            session.execute(
                text(
                    "INSERT INTO source_connection_versions "
                    "(connection_id, version, owner_id, auth_kind, secret_ref, created_by, "
                    "created_at) VALUES (:id, 4, :owner_id, 'browser_state', :reference, "
                    ":owner_id, :now)"
                ),
                {
                    "id": connection_id,
                    "owner_id": owner_id,
                    "reference": invalid_reference,
                    "now": now,
                },
            )


def test_browser_state_maintenance_rotates_disables_and_requires_new_capture(
    source_connection_client: TestClient, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    client = source_connection_client
    owner_id = UUID(_initialize(client))
    connection_id = uuid4()
    root = tmp_path / "browser-states"
    root.mkdir(mode=0o700)
    store = BrowserStateStore(root)
    original = {"cookies": [{"name": "fixture", "value": "one"}], "origins": []}
    reference = store.save(
        owner_id=owner_id, connection_id=connection_id, version=1, state=original
    )
    now = datetime.now(UTC)
    factory = client.app.state.session_factory
    with factory() as session, session.begin():
        session.execute(
            text(
                "INSERT INTO source_connections "
                "(id, owner_id, source_key, status, current_version, created_at, updated_at) "
                "VALUES (:id, :owner_id, 'browser_fixture', 'active', 1, :now, :now)"
            ),
            {"id": connection_id, "owner_id": owner_id, "now": now},
        )
        session.execute(
            text(
                "INSERT INTO source_connection_versions "
                "(connection_id, version, owner_id, auth_kind, secret_ref, created_by, "
                "created_at) VALUES (:id, 1, :owner_id, 'browser_state', :reference, "
                ":owner_id, :now)"
            ),
            {"id": connection_id, "owner_id": owner_id, "reference": reference, "now": now},
        )

    replacement = {"cookies": [{"name": "fixture", "value": "two"}], "origins": []}
    with factory() as session:
        rotated = SourceConnectionService(session).rotate_browser_state(
            owner_id=owner_id,
            connection_id=connection_id,
            expected_version=1,
            state=replacement,
            store=store,
        )
    assert rotated.version == 2 and rotated.status is SourceConnectionStatus.ACTIVE
    with factory() as session, session.begin():
        with pytest.raises(ApplicationError, match="connection_version_conflict"):
            require_browser_state_execution(
                session,
                owner_id=owner_id,
                connection_id=connection_id,
                connection_version=1,
                store=store,
            )
        assert (
            require_browser_state_execution(
                session,
                owner_id=owner_id,
                connection_id=connection_id,
                connection_version=2,
                store=store,
            )
            == replacement
        )

    with factory() as session:
        with pytest.raises(ApplicationError, match="connection_version_conflict"):
            SourceConnectionService(session).rotate_browser_state(
                owner_id=owner_id,
                connection_id=connection_id,
                expected_version=1,
                state=replacement,
                store=store,
            )
        disabled = SourceConnectionService(session).disable_browser_state(
            owner_id=owner_id,
            connection_id=connection_id,
            expected_version=2,
        )
    assert disabled.version == 3 and disabled.status is SourceConnectionStatus.DISABLED
    with pytest.raises(BrowserStateError, match="file_invalid"):
        store.load(
            owner_id=owner_id,
            connection_id=connection_id,
            version=3,
            reference=store.reference(owner_id, connection_id, 3),
        )
    resumed_state = {"cookies": [{"name": "fixture", "value": "three"}], "origins": []}
    store.save(owner_id=owner_id, connection_id=connection_id, version=4, state=resumed_state)
    with factory() as session:
        repeated = SourceConnectionService(session).disable_browser_state(
            owner_id=owner_id,
            connection_id=connection_id,
            expected_version=3,
        )
        reenabled = SourceConnectionService(session).rotate_browser_state(
            owner_id=owner_id,
            connection_id=connection_id,
            expected_version=3,
            state=resumed_state,
            store=store,
        )
    assert repeated.version == 3
    assert reenabled.version == 4 and reenabled.status is SourceConnectionStatus.ACTIVE

    capture_directory = tmp_path / "capture"
    capture_directory.mkdir(mode=0o700)
    capture = capture_directory / "storage-state.json"
    capture.write_text(
        '{"cookies":[{"name":"fixture","value":"CLI_SECRET_MARKER"}],"origins":[]}',
        encoding="utf-8",
    )
    capture.chmod(0o600)
    monkeypatch.setenv("HOTKEY_DATABASE_URL", os.environ["HOTKEY_TEST_DATABASE_URL"])
    monkeypatch.setenv("HOTKEY_BROWSER_STATE_DIR", str(root))
    monkeypatch.setenv("HOTKEY_ENVIRONMENT", "test")
    get_settings.cache_clear()
    try:
        capture.chmod(0o644)
        invalid_capture = CliRunner().invoke(
            cli_app,
            [
                "connections",
                "rotate-browser-state",
                "--owner-id",
                str(owner_id),
                "--connection-id",
                str(connection_id),
                "--expected-version",
                "4",
                "--capture-file",
                str(capture),
            ],
        )
        capture.chmod(0o600)
        rotate_result = CliRunner().invoke(
            cli_app,
            [
                "connections",
                "rotate-browser-state",
                "--owner-id",
                str(owner_id),
                "--connection-id",
                str(connection_id),
                "--expected-version",
                "4",
                "--capture-file",
                str(capture),
            ],
        )
        disable_result = CliRunner().invoke(
            cli_app,
            [
                "connections",
                "disable-browser-state",
                "--owner-id",
                str(owner_id),
                "--connection-id",
                str(connection_id),
                "--expected-version",
                "5",
            ],
        )
    finally:
        get_settings.cache_clear()
    assert invalid_capture.exit_code == 1
    assert "capture_invalid" in invalid_capture.output
    assert "CLI_SECRET_MARKER" not in invalid_capture.output
    assert str(capture) not in invalid_capture.output
    assert rotate_result.exit_code == 0, rotate_result.output
    assert disable_result.exit_code == 0, disable_result.output
    assert "CLI_SECRET_MARKER" not in rotate_result.output
    assert str(capture) not in rotate_result.output
    assert "version: 5" in rotate_result.output
    assert "version: 6" in disable_result.output
    with (
        factory() as session,
        session.begin(),
        pytest.raises(ApplicationError, match="connection_disabled"),
    ):
        require_browser_state_execution(
            session,
            owner_id=owner_id,
            connection_id=connection_id,
            connection_version=6,
            store=store,
        )
