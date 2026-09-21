from __future__ import annotations

import os
from collections.abc import Iterator

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text

from core.config import Settings
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
        connection.execute(text("TRUNCATE identity_sessions, identity_users"))
    try:
        with TestClient(create_app(settings)) as client:
            yield client
    finally:
        with engine.begin() as connection:
            connection.execute(text("TRUNCATE identity_sessions, identity_users"))
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
