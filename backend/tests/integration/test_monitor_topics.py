from __future__ import annotations

import os
from collections.abc import Iterator
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text

from core.config import Settings
from core.errors import ApplicationError
from main import create_app
from monitors.services import MonitorTopicService

_BOOTSTRAP_TOKEN = "monitor-topics-isolated-bootstrap-token"
_PASSWORD = "correct horse battery staple"
_TRUNCATE = (
    "TRUNCATE provenance_manifest_inputs, provenance_manifests, "
    "evidence_cleanup_targets, evidence_deletions, "
    "evidence_resources, evidence_retention_policies, source_access_policies, "
    "resource_budget_reservations, resource_budget_windows, resource_budget_policies, "
    "resource_usage_attempts, resource_component_policies, job_stage_attempts, "
    "processed_messages, job_attempts, outbox_messages, jobs, "
    "monitor_topic_versions, monitor_topics, identity_sessions, identity_users"
)


@pytest.fixture
def monitor_topic_client() -> Iterator[TestClient]:
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


def _initialize(client: TestClient) -> None:
    response = client.post(
        "/api/identity/initialize",
        headers={
            "X-HotKey-Bootstrap-Token": _BOOTSTRAP_TOKEN,
            "X-HotKey-CSRF": "1",
        },
        json={"username": "topic-owner", "password": _PASSWORD},
    )
    assert response.status_code == 201


def _csrf_headers(client: TestClient) -> dict[str, str]:
    return {"X-HotKey-CSRF": client.cookies["hotkey_csrf"]}


def _topic_payload() -> dict[str, object]:
    return {
        "name": "  品牌\t召回  ",
        "match_any": ["Brand", "\uff22\uff32\uff21\uff2e\uff24"],
        "match_all": ["召回"],
        "exclude": ["招聘"],
    }


def test_topic_create_edit_and_reopen_preserve_immutable_versions(
    monitor_topic_client: TestClient,
) -> None:
    _initialize(monitor_topic_client)

    created = monitor_topic_client.post(
        "/api/topics",
        headers=_csrf_headers(monitor_topic_client),
        json=_topic_payload(),
    )

    assert created.status_code == 201, created.json()
    assert created.headers["location"] == f"/api/topics/{created.json()['id']}"
    assert created.headers["cache-control"] == "no-store"
    assert created.json()["name"] == "品牌 召回"
    assert created.json()["status"] == "paused"
    assert created.json()["readiness_status"] == "pending_source_selection"
    assert created.json()["current_version"] == 1
    assert created.json()["rules"] == {
        "match_any": ["Brand"],
        "match_all": ["召回"],
        "exclude": ["招聘"],
    }
    assert "owner_id" not in created.json()

    updated = monitor_topic_client.patch(
        created.headers["location"],
        headers=_csrf_headers(monitor_topic_client),
        json={
            **_topic_payload(),
            "expected_version": 1,
            "match_any": ["Brand", "厂商"],
        },
    )

    assert updated.status_code == 200, updated.json()
    assert updated.json()["current_version"] == 2
    reopened = monitor_topic_client.get(created.headers["location"])
    assert reopened.status_code == 200
    assert reopened.json() == updated.json()

    factory = monitor_topic_client.app.state.session_factory
    with factory() as session:
        versions = session.execute(
            text(
                "SELECT version, match_any FROM monitor_topic_versions "
                "WHERE topic_id = :topic_id ORDER BY version"
            ),
            {"topic_id": created.json()["id"]},
        ).all()
    assert versions == [(1, ["Brand"]), (2, ["Brand", "厂商"])]


def test_stale_topic_edit_conflicts_without_partial_version(
    monitor_topic_client: TestClient,
) -> None:
    _initialize(monitor_topic_client)
    created = monitor_topic_client.post(
        "/api/topics",
        headers=_csrf_headers(monitor_topic_client),
        json=_topic_payload(),
    )
    location = created.headers["location"]

    first = monitor_topic_client.patch(
        location,
        headers=_csrf_headers(monitor_topic_client),
        json={**_topic_payload(), "expected_version": 1, "exclude": ["招聘", "校招"]},
    )
    stale = monitor_topic_client.patch(
        location,
        headers=_csrf_headers(monitor_topic_client),
        json={**_topic_payload(), "expected_version": 1, "exclude": ["广告"]},
    )

    assert first.status_code == 200
    assert stale.status_code == 409
    assert stale.json()["code"] == "topic_version_conflict"
    factory = monitor_topic_client.app.state.session_factory
    with factory() as session:
        assert (
            session.execute(
                text("SELECT count(*) FROM monitor_topic_versions WHERE topic_id = :topic_id"),
                {"topic_id": created.json()["id"]},
            ).scalar_one()
            == 2
        )


def test_name_only_edit_keeps_rule_version_and_owner_filter_hides_topic(
    monitor_topic_client: TestClient,
) -> None:
    _initialize(monitor_topic_client)
    created = monitor_topic_client.post(
        "/api/topics",
        headers=_csrf_headers(monitor_topic_client),
        json=_topic_payload(),
    )

    renamed = monitor_topic_client.patch(
        created.headers["location"],
        headers=_csrf_headers(monitor_topic_client),
        json={**_topic_payload(), "name": "品牌召回监控", "expected_version": 1},
    )

    assert renamed.status_code == 200
    assert renamed.json()["name"] == "品牌召回监控"
    assert renamed.json()["current_version"] == 1
    factory = monitor_topic_client.app.state.session_factory
    with factory() as session:
        assert (
            session.execute(
                text("SELECT count(*) FROM monitor_topic_versions WHERE topic_id = :topic_id"),
                {"topic_id": created.json()["id"]},
            ).scalar_one()
            == 1
        )
        with pytest.raises(ApplicationError, match="resource_not_found"):
            MonitorTopicService(session).get_topic(
                owner_id=uuid4(),
                topic_id=created.json()["id"],
            )


def test_conflicting_keyword_groups_are_rejected_without_persistence(
    monitor_topic_client: TestClient,
) -> None:
    _initialize(monitor_topic_client)

    rejected = monitor_topic_client.post(
        "/api/topics",
        headers=_csrf_headers(monitor_topic_client),
        json={**_topic_payload(), "exclude": [" brand "]},
    )

    assert rejected.status_code == 422
    assert rejected.json()["code"] == "keyword_group_conflict"
    factory = monitor_topic_client.app.state.session_factory
    with factory() as session:
        assert session.execute(text("SELECT count(*) FROM monitor_topics")).scalar_one() == 0


def test_topic_routes_enforce_session_csrf_and_hidden_missing_boundary(
    monitor_topic_client: TestClient,
) -> None:
    missing_session = monitor_topic_client.post(
        "/api/topics",
        headers={"X-HotKey-CSRF": "1"},
        json=_topic_payload(),
    )
    assert missing_session.status_code == 401

    _initialize(monitor_topic_client)
    missing_csrf = monitor_topic_client.post("/api/topics", json=_topic_payload())
    assert missing_csrf.status_code == 403
    missing = monitor_topic_client.get(f"/api/topics/{uuid4()}")
    assert missing.status_code == 404
    assert missing.json()["code"] == "resource_not_found"
