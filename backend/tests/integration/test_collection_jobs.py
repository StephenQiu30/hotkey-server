from __future__ import annotations

import os
from collections.abc import Iterator
from uuid import UUID, uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text

from core.config import Settings
from main import create_app

_BOOTSTRAP_TOKEN = "bootstrap-token-used-only-by-the-isolated-test"
_PASSWORD = "correct horse battery staple"
_TRUNCATE = (
    "TRUNCATE provenance_manifest_inputs, provenance_manifests, "
    "evidence_cleanup_targets, evidence_deletions, "
    "evidence_resources, evidence_retention_policies, source_access_policies, "
    "resource_budget_reservations, resource_budget_windows, resource_budget_policies, "
    "resource_usage_attempts, resource_component_policies, job_stage_attempts, "
    "processed_messages, job_attempts, outbox_messages, jobs, "
    "identity_sessions, identity_users"
)


@pytest.fixture
def collection_job_client() -> Iterator[TestClient]:
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
        json={"username": "owner", "password": _PASSWORD},
    )
    assert response.status_code == 201


def _payload(*, operation_id: UUID | None = None, window: int = 7) -> dict[str, object]:
    return {
        "operation_id": str(operation_id or uuid4()),
        "kind": "monitor.collect",
        "observation": {
            "configuration_ref": "monitor-config-1",
            "configuration_version": 3,
            "source_key": "x",
            "source_capability": "search",
        },
        "scheduled_for_at": None,
        "scope": {"source_id": "account-1", "window": window},
    }


def _csrf_headers(client: TestClient) -> dict[str, str]:
    return {"X-HotKey-CSRF": client.cookies["hotkey_csrf"]}


def test_submitted_job_is_persisted_and_readable_after_refresh(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    payload = _payload()

    accepted = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=payload,
    )

    assert accepted.status_code == 202, accepted.json()
    assert accepted.json()["status"] == "queued"
    assert set(accepted.json()) == {"job_id", "status"}
    location = accepted.headers["location"]
    assert location == f"/api/jobs/{accepted.json()['job_id']}"
    assert accepted.headers["cache-control"] == "no-store"

    refreshed = collection_job_client.get(location)

    assert refreshed.status_code == 200
    assert refreshed.headers["cache-control"] == "no-store"
    assert refreshed.json() == {
        "id": accepted.json()["job_id"],
        "operation_id": payload["operation_id"],
        "kind": "monitor.collect",
        "observation": payload["observation"],
        "status": "queued",
        "scheduled_for_at": None,
        "started_at": None,
        "completed_at": None,
        "created_at": refreshed.json()["created_at"],
    }
    assert "owner_id" not in refreshed.json()
    assert "scope" not in refreshed.json()


def test_repeated_submission_returns_one_persisted_job_and_outbox(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    payload = _payload()

    responses = [
        collection_job_client.post(
            "/api/jobs",
            headers=_csrf_headers(collection_job_client),
            json=payload,
        )
        for _ in range(3)
    ]

    assert [response.status_code for response in responses] == [202, 202, 202], [
        response.json() for response in responses
    ]
    assert len({response.json()["job_id"] for response in responses}) == 1
    factory = collection_job_client.app.state.session_factory
    with factory() as session:
        assert session.execute(text("SELECT count(*) FROM jobs")).scalar_one() == 1
        assert session.execute(text("SELECT count(*) FROM outbox_messages")).scalar_one() == 1


def test_conflicting_reuse_of_operation_id_preserves_original_job(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    operation_id = uuid4()
    original = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=_payload(operation_id=operation_id, window=7),
    )
    conflicting = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=_payload(operation_id=operation_id, window=8),
    )

    assert original.status_code == 202, original.json()
    assert conflicting.status_code == 409
    assert conflicting.json()["code"] == "idempotency_conflict"
    factory = collection_job_client.app.state.session_factory
    with factory() as session:
        row = session.execute(text("SELECT id, scope FROM jobs")).one()
        assert str(row.id) == original.json()["job_id"]
        assert row.scope["window"] == 7
        assert session.execute(text("SELECT count(*) FROM outbox_messages")).scalar_one() == 1


def test_job_routes_enforce_session_csrf_and_missing_resource_boundaries(
    collection_job_client: TestClient,
) -> None:
    payload = _payload()
    missing_session = collection_job_client.post(
        "/api/jobs",
        headers={"X-HotKey-CSRF": "1"},
        json=payload,
    )
    assert missing_session.status_code == 401
    assert missing_session.json()["code"] == "invalid_session"

    _initialize(collection_job_client)
    missing_csrf = collection_job_client.post("/api/jobs", json=payload)
    assert missing_csrf.status_code == 403
    assert missing_csrf.json()["code"] == "csrf_invalid"

    missing_job = collection_job_client.get(f"/api/jobs/{uuid4()}")
    assert missing_job.status_code == 404
    assert missing_job.json()["code"] == "resource_not_found"

    collection_job_client.cookies.set("hotkey_session", "forged-session")
    forged_session = collection_job_client.get(f"/api/jobs/{uuid4()}")
    assert forged_session.status_code == 401
    assert forged_session.json()["code"] == "invalid_session"


def test_public_submission_rejects_unregistered_job_kinds(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    payload = _payload()
    payload["kind"] = "arbitrary.command"

    response = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=payload,
    )

    assert response.status_code == 422
    assert response.json()["code"] == "validation_error"
    factory = collection_job_client.app.state.session_factory
    with factory() as session:
        assert session.execute(text("SELECT count(*) FROM jobs")).scalar_one() == 0
        assert session.execute(text("SELECT count(*) FROM outbox_messages")).scalar_one() == 0


def test_job_openapi_contract_is_generated_from_runtime_routes(
    collection_job_client: TestClient,
) -> None:
    schema = collection_job_client.get("/openapi.json").json()
    create_operation = schema["paths"]["/api/jobs"]["post"]
    get_operation = schema["paths"]["/api/jobs/{job_id}"]["get"]

    assert create_operation["operationId"] == "createCollectionJob"
    assert "200" not in create_operation["responses"]
    assert create_operation["responses"]["202"]["content"]["application/json"]["schema"] == {
        "$ref": "#/components/schemas/JobAcceptedView"
    }
    assert get_operation["operationId"] == "getCollectionJob"
    assert get_operation["responses"]["200"]["content"]["application/json"]["schema"] == {
        "$ref": "#/components/schemas/JobStatusView"
    }
    assert create_operation["security"] == [{"SessionCookie": []}]
    assert get_operation["security"] == [{"SessionCookie": []}]
    for status_code in ("401", "403", "409", "422", "500"):
        assert create_operation["responses"][status_code]["content"]["application/json"][
            "schema"
        ] == {"$ref": "#/components/schemas/ErrorView"}
