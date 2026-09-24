from __future__ import annotations

import os
from collections.abc import Iterator
from dataclasses import replace
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text

from api.dependencies import require_identity_session
from core.config import Settings
from identity.services import IdentityService
from jobs.schemas import JobAcceptanceInput, JobObservationContext
from jobs.services import JobService
from main import create_app

_BOOTSTRAP_TOKEN = "bootstrap-token-used-only-by-the-isolated-test"
_PASSWORD = "correct horse battery staple"


@pytest.fixture
def owner_client() -> Iterator[TestClient]:
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
        connection.execute(
            text(
                "TRUNCATE content_version_relations, content_visibility_observations, "
                "content_observations, content_versions, "
                "content_discoveries, content_records, "
                "source_capability_evidence, source_connection_versions, "
                "source_connections, provenance_manifest_inputs, provenance_manifests, "
                "evidence_cleanup_targets, evidence_deletions, "
                "evidence_resources, evidence_retention_policies, source_access_policies, "
                "resource_budget_reservations, resource_budget_windows, "
                "resource_budget_policies, resource_usage_attempts, "
                "resource_component_policies, "
                "job_stage_attempts, processed_messages, job_attempts, "
                "outbox_messages, coverage_windows, "
                "jobs, followed_account_aliases, followed_accounts, "
                "monitor_topic_versions, monitor_topics, "
                "identity_sessions, identity_users"
            )
        )
    try:
        with TestClient(create_app(settings)) as client:
            yield client
    finally:
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE content_version_relations, content_visibility_observations, "
                    "content_observations, content_versions, "
                    "content_discoveries, content_records, "
                    "source_capability_evidence, source_connection_versions, "
                    "source_connections, provenance_manifest_inputs, provenance_manifests, "
                    "evidence_cleanup_targets, evidence_deletions, "
                    "evidence_resources, evidence_retention_policies, source_access_policies, "
                    "resource_budget_reservations, resource_budget_windows, "
                    "resource_budget_policies, resource_usage_attempts, "
                    "resource_component_policies, "
                    "job_stage_attempts, processed_messages, job_attempts, "
                    "outbox_messages, coverage_windows, "
                    "jobs, followed_account_aliases, followed_accounts, "
                    "monitor_topic_versions, monitor_topics, "
                    "identity_sessions, identity_users"
                )
            )
        engine.dispose()


def test_owner_can_open_private_workspace(owner_client: TestClient) -> None:
    initialized = owner_client.post(
        "/api/identity/initialize",
        headers={
            "X-HotKey-Bootstrap-Token": _BOOTSTRAP_TOKEN,
            "X-HotKey-CSRF": "1",
        },
        json={"username": "owner", "password": _PASSWORD},
    )
    assert initialized.status_code == 201

    workspace = owner_client.get("/api/identity/workspace")

    assert workspace.status_code == 200
    assert workspace.json() == {"owner": initialized.json()["user"]}
    assert workspace.headers["cache-control"] == "no-store"
    assert "password" not in workspace.text


def test_missing_and_forged_sessions_cannot_open_private_workspace(
    owner_client: TestClient,
) -> None:
    missing = owner_client.get("/api/identity/workspace")
    owner_client.cookies.set("hotkey_session", "forged-session-token")
    forged = owner_client.get("/api/identity/workspace")

    assert missing.status_code == forged.status_code == 401
    assert missing.json()["code"] == forged.json()["code"] == "invalid_session"
    assert set(missing.json()) == set(forged.json()) == {"code", "message", "request_id"}


def _initialize(client: TestClient) -> None:
    response = client.post(
        "/api/identity/initialize",
        headers={"X-HotKey-Bootstrap-Token": _BOOTSTRAP_TOKEN, "X-HotKey-CSRF": "1"},
        json={"username": "owner", "password": _PASSWORD},
    )
    assert response.status_code == 201


@pytest.mark.parametrize(
    "method,path,payload",
    [
        ("GET", "/api/topics/{topic_id}", None),
        (
            "PATCH",
            "/api/topics/{topic_id}",
            {
                "name": "changed",
                "match_any": ["term"],
                "match_all": [],
                "exclude": [],
                "expected_version": 1,
            },
        ),
        ("POST", "/api/topics/{topic_id}/clone", None),
        ("POST", "/api/topics/{topic_id}/pause", None),
        ("POST", "/api/topics/{topic_id}/resume", None),
        ("POST", "/api/topics/{topic_id}/archive", None),
        ("GET", "/api/jobs/{job_id}", None),
        ("POST", "/api/jobs/{job_id}/cancel", None),
        ("POST", "/api/jobs/{job_id}/retry", None),
    ],
)
def test_external_actor_cannot_read_or_mutate_known_resources(
    owner_client: TestClient, method: str, path: str, payload: dict | None
) -> None:
    _initialize(owner_client)
    headers = {"X-HotKey-CSRF": owner_client.cookies["hotkey_csrf"]}
    topic = owner_client.post(
        "/api/topics",
        headers=headers,
        json={"name": "private topic", "match_any": ["private"], "match_all": [], "exclude": []},
    )
    factory = owner_client.app.state.session_factory
    with factory() as session:
        owner_id = session.execute(
            text("SELECT id FROM identity_users WHERE username = 'owner'")
        ).scalar_one()
        job = JobService(session).accept(
            owner_id=owner_id,
            command=JobAcceptanceInput(
                operation_id=uuid4(),
                kind="monitor.collect",
                observation=JobObservationContext(
                    configuration_ref="private-config",
                    configuration_version=1,
                ),
                scope={},
            ),
        )
        identity = IdentityService(session, owner_client.app.state.settings).authenticate(
            owner_client.cookies["hotkey_session"]
        )
    assert topic.status_code == 201
    topic_id, job_id = topic.json()["id"], str(job.id)
    foreign = replace(
        identity,
        view=identity.view.model_copy(
            update={"user": identity.view.user.model_copy(update={"id": uuid4()})}
        ),
    )
    owner_client.app.dependency_overrides[require_identity_session] = lambda: foreign
    try:
        denied = owner_client.request(
            method, path.format(topic_id=topic_id, job_id=job_id), headers=headers, json=payload
        )
        absent = owner_client.request(
            method, path.format(topic_id=uuid4(), job_id=uuid4()), headers=headers, json=payload
        )
        assert denied.status_code == absent.status_code == 404
        assert denied.json()["code"] == absent.json()["code"] == "resource_not_found"
        assert denied.json()["message"] == absent.json()["message"]
        assert "private" not in denied.text
        assert owner_client.get("/api/topics").json() == {"items": [], "next_cursor": None}
        assert owner_client.get("/api/contents").json() == {"items": [], "next_cursor": None}
        assert denied.headers.get("cache-control") == "no-store"
    finally:
        owner_client.app.dependency_overrides.clear()
    assert owner_client.get(f"/api/topics/{topic_id}").json() == topic.json()
    assert owner_client.get(f"/api/jobs/{job_id}").json()["status"] == "queued"
    with factory() as session:
        assert session.scalar(text("SELECT count(*) FROM monitor_topic_versions")) == 1
        assert session.scalar(text("SELECT count(*) FROM outbox_messages")) == 1


@pytest.mark.parametrize(
    "path",
    [
        "/api/topics",
        "/api/contents",
        "/api/source-capabilities",
        "/api/jobs/00000000-0000-0000-0000-000000000000",
    ],
)
def test_private_errors_are_not_cacheable_and_revoked_session_stays_denied(
    owner_client: TestClient, path: str
) -> None:
    missing = owner_client.get(path)
    assert missing.status_code == 401
    assert missing.headers.get("cache-control") == "no-store"
    _initialize(owner_client)
    token = owner_client.cookies["hotkey_session"]
    assert (
        owner_client.delete(
            "/api/identity/session", headers={"X-HotKey-CSRF": owner_client.cookies["hotkey_csrf"]}
        ).status_code
        == 204
    )
    owner_client.cookies.set("hotkey_session", token)
    revoked = owner_client.get(path)
    assert revoked.status_code == 401
    assert revoked.headers.get("cache-control") == "no-store"
