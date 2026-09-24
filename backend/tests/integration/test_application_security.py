from __future__ import annotations

import os
from collections.abc import Iterator

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text
from typer.testing import CliRunner

from cli.commands import app as cli_app
from core.config import Settings, get_settings
from main import create_app

BOOTSTRAP_TOKEN = "bootstrap-token-used-only-by-the-isolated-test"
TEST_PASSWORD = "correct horse battery staple"


@pytest.fixture
def identity_client() -> Iterator[TestClient]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    settings = Settings(
        environment="test",
        log_level="WARNING",
        database_url=database_url,
        bootstrap_token=BOOTSTRAP_TOKEN,
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


def _initialize(client: TestClient, password: str = TEST_PASSWORD):
    return client.post(
        "/api/identity/initialize",
        headers={
            "X-HotKey-Bootstrap-Token": BOOTSTRAP_TOKEN,
            "X-HotKey-CSRF": "1",
        },
        json={"username": "owner", "password": password},
    )


def test_first_owner_initialization_is_controlled_and_creates_a_session(
    identity_client: TestClient,
) -> None:
    payload = {"username": "owner", "password": TEST_PASSWORD}

    rejected = identity_client.post(
        "/api/identity/initialize",
        headers={"X-HotKey-Bootstrap-Token": "wrong", "X-HotKey-CSRF": "1"},
        json=payload,
    )
    created = _initialize(identity_client)

    assert rejected.status_code == 403
    assert rejected.json()["code"] == "bootstrap_forbidden"
    assert created.status_code == 201
    assert created.json()["user"]["username"] == "owner"
    assert created.json()["expires_at"]
    assert created.cookies.get("hotkey_session")
    assert created.cookies.get("hotkey_csrf")
    assert "password" not in created.text
    assert BOOTSTRAP_TOKEN not in created.text
    session_cookie = next(
        value
        for value in created.headers.get_list("set-cookie")
        if value.startswith("hotkey_session=")
    )
    assert "HttpOnly" in session_cookie
    assert "SameSite=strict" in session_cookie
    assert "Secure" not in session_cookie

    repeated = _initialize(identity_client)
    assert repeated.status_code == 409
    assert repeated.json()["code"] == "identity_already_initialized"


def test_login_protected_session_and_logout_revoke_the_old_cookie(
    identity_client: TestClient,
) -> None:
    assert _initialize(identity_client).status_code == 201

    wrong_username = identity_client.post(
        "/api/identity/sessions",
        headers={"X-HotKey-CSRF": "1"},
        json={"username": "unknown", "password": TEST_PASSWORD},
    )
    wrong_password = identity_client.post(
        "/api/identity/sessions",
        headers={"X-HotKey-CSRF": "1"},
        json={"username": "owner", "password": "this password is not correct"},
    )
    assert wrong_username.status_code == wrong_password.status_code == 401
    assert wrong_username.json()["code"] == wrong_password.json()["code"] == "invalid_credentials"

    login = identity_client.post(
        "/api/identity/sessions",
        headers={"X-HotKey-CSRF": "1"},
        json={"username": "OWNER", "password": TEST_PASSWORD},
    )
    assert login.status_code == 200
    old_session = identity_client.cookies["hotkey_session"]
    csrf_token = identity_client.cookies["hotkey_csrf"]
    assert identity_client.get("/api/identity/session").status_code == 200

    missing_csrf = identity_client.delete("/api/identity/session")
    wrong_csrf = identity_client.delete(
        "/api/identity/session",
        headers={"X-HotKey-CSRF": "wrong"},
    )
    assert missing_csrf.status_code == wrong_csrf.status_code == 403
    assert missing_csrf.json()["code"] == wrong_csrf.json()["code"] == "csrf_invalid"

    logout = identity_client.delete(
        "/api/identity/session",
        headers={"X-HotKey-CSRF": csrf_token},
    )
    assert logout.status_code == 204
    identity_client.cookies.set("hotkey_session", old_session)
    assert identity_client.get("/api/identity/session").status_code == 401


def test_password_recovery_cli_revokes_sessions_without_echoing_the_password(
    identity_client: TestClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    assert _initialize(identity_client).status_code == 201
    old_session = identity_client.cookies["hotkey_session"]
    new_password = "replacement password only for recovery"
    database_url = os.environ["HOTKEY_TEST_DATABASE_URL"]
    monkeypatch.setenv("HOTKEY_DATABASE_URL", database_url)
    monkeypatch.setenv("HOTKEY_ENVIRONMENT", "test")
    get_settings.cache_clear()
    try:
        runner = CliRunner()
        rejected = runner.invoke(
            cli_app,
            ["identity", "reset-password"],
            input="too short\ntoo short\n",
        )
        result = runner.invoke(
            cli_app,
            ["identity", "reset-password"],
            input=f"{new_password}\n{new_password}\n",
        )
    finally:
        get_settings.cache_clear()

    assert rejected.exit_code == 1
    assert "invalid_password" in rejected.stderr
    assert result.exit_code == 0
    assert "revoked sessions:" in result.stdout
    assert new_password not in result.stdout
    assert identity_client.cookies["hotkey_session"] == old_session
    assert identity_client.get("/api/identity/session").status_code == 401

    old_login = identity_client.post(
        "/api/identity/sessions",
        headers={"X-HotKey-CSRF": "1"},
        json={"username": "owner", "password": TEST_PASSWORD},
    )
    new_login = identity_client.post(
        "/api/identity/sessions",
        headers={"X-HotKey-CSRF": "1"},
        json={"username": "owner", "password": new_password},
    )
    assert old_login.status_code == 401
    assert new_login.status_code == 200


def test_database_stores_only_password_and_session_digests(identity_client: TestClient) -> None:
    created = _initialize(identity_client)
    assert created.status_code == 201
    session_token = identity_client.cookies["hotkey_session"]
    csrf_token = identity_client.cookies["hotkey_csrf"]

    factory = identity_client.app.state.session_factory
    with factory() as session:
        row = session.execute(
            text(
                "SELECT u.password_hash, s.token_digest, s.csrf_digest "
                "FROM identity_users u JOIN identity_sessions s ON s.user_id = u.id"
            )
        ).one()

    assert row.password_hash.startswith("$argon2")
    assert TEST_PASSWORD not in row.password_hash
    assert len(row.token_digest) == len(row.csrf_digest) == 32
    assert session_token.encode() not in {row.token_digest, row.csrf_digest}
    assert csrf_token.encode() not in {row.token_digest, row.csrf_digest}


def test_public_mutations_require_csrf_and_initialization_configuration(
    identity_client: TestClient,
) -> None:
    missing_csrf = identity_client.post(
        "/api/identity/initialize",
        headers={"X-HotKey-Bootstrap-Token": BOOTSTRAP_TOKEN},
        json={"username": "owner", "password": TEST_PASSWORD},
    )
    assert missing_csrf.status_code == 403
    assert missing_csrf.json()["code"] == "csrf_invalid"

    settings = Settings(
        environment="test",
        log_level="WARNING",
        database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
    )
    with TestClient(create_app(settings)) as unconfigured:
        response = unconfigured.post(
            "/api/identity/initialize",
            headers={
                "X-HotKey-Bootstrap-Token": BOOTSTRAP_TOKEN,
                "X-HotKey-CSRF": "1",
            },
            json={"username": "owner", "password": TEST_PASSWORD},
        )
    assert response.status_code == 403
    assert response.json()["code"] == "bootstrap_forbidden"
