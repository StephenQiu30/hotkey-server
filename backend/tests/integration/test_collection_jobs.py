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
    "TRUNCATE content_version_relations, content_visibility_observations, "
    "content_observations, content_versions, "
    "content_discoveries, content_records, "
    "source_capability_evidence, source_connection_versions, "
    "source_connections, provenance_manifest_inputs, provenance_manifests, "
    "evidence_cleanup_targets, evidence_deletions, "
    "evidence_resources, evidence_retention_policies, source_access_policies, "
    "resource_budget_reservations, resource_budget_windows, resource_budget_policies, "
    "resource_usage_attempts, resource_component_policies, job_stage_attempts, "
    "processed_messages, job_attempts, "
    "outbox_messages, coverage_windows, "
    "jobs, monitor_topic_versions, monitor_topics, "
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
        "progress": {
            "stage": None,
            "requests_sent": 0,
            "items_saved": 0,
            "updated_at": None,
        },
        "cancellation": None,
        "failure": None,
        "result_content_id": None,
        "retry_count": 0,
        "next_run_at": None,
        "scheduled_for_at": None,
        "started_at": None,
        "completed_at": None,
        "created_at": refreshed.json()["created_at"],
    }
    assert "owner_id" not in refreshed.json()
    assert "scope" not in refreshed.json()


def test_webpage_submission_derives_connection_context_without_leaking_url_to_outbox(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    configured = collection_job_client.put(
        "/api/source-connections/web",
        headers=_csrf_headers(collection_job_client),
        json={
            "expected_version": 0,
            "status": "active",
            "allowed_hosts": ["example.com"],
        },
    )
    assert configured.status_code == 200, configured.json()
    operation_id = uuid4()

    accepted = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json={
            "operation_id": str(operation_id),
            "kind": "webpage.collect",
            "url": "https://EXAMPLE.com/Articles/One?q=1#ignored",
        },
    )

    assert accepted.status_code == 202, accepted.json()
    job_id = accepted.json()["job_id"]
    refreshed = collection_job_client.get(f"/api/jobs/{job_id}")
    assert refreshed.status_code == 200
    assert refreshed.json()["kind"] == "webpage.collect"
    assert refreshed.json()["observation"] == {
        "configuration_ref": f"connection:{configured.json()['id'].replace('-', '')}",
        "configuration_version": 1,
        "source_key": "web",
        "source_capability": "page_content",
    }
    assert refreshed.json()["result_content_id"] is None

    factory = collection_job_client.app.state.session_factory
    with factory() as session:
        job = session.execute(
            text(
                "SELECT scope, configuration_ref, configuration_version, source_key, "
                "source_capability FROM jobs WHERE id = :job_id"
            ),
            {"job_id": job_id},
        ).one()
        outbox_payload = session.execute(
            text("SELECT payload FROM outbox_messages WHERE aggregate_id = :job_id"),
            {"job_id": job_id},
        ).scalar_one()
    assert job.scope == {
        "connection_id": configured.json()["id"],
        "entry_point": "manual",
        "target_url": "https://example.com/Articles/One?q=1",
    }
    assert job.configuration_ref == f"connection:{configured.json()['id'].replace('-', '')}"
    assert job.configuration_version == 1
    assert job.source_key == "web"
    assert job.source_capability == "page_content"
    assert "target_url" not in outbox_payload


def test_webpage_submission_rejects_client_owned_execution_context(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)

    response = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json={
            "operation_id": str(uuid4()),
            "kind": "webpage.collect",
            "url": "https://example.com/article",
            "scope": {"connection_id": str(uuid4())},
            "observation": {
                "configuration_ref": "attacker-controlled",
                "configuration_version": 99,
                "source_key": "web",
                "source_capability": "page_content",
            },
        },
    )

    assert response.status_code == 422
    factory = collection_job_client.app.state.session_factory
    with factory() as session:
        assert session.execute(text("SELECT count(*) FROM jobs")).scalar_one() == 0


def test_webpage_lost_response_replay_survives_connection_replacement(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    configured = collection_job_client.put(
        "/api/source-connections/web",
        headers=_csrf_headers(collection_job_client),
        json={
            "expected_version": 0,
            "status": "active",
            "allowed_hosts": ["example.com"],
        },
    )
    assert configured.status_code == 200
    operation_id = uuid4()
    payload = {
        "operation_id": str(operation_id),
        "kind": "webpage.collect",
        "url": "https://example.com/article#first",
    }
    original = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=payload,
    )
    replaced = collection_job_client.put(
        "/api/source-connections/web",
        headers=_csrf_headers(collection_job_client),
        json={
            "expected_version": 1,
            "status": "active",
            "allowed_hosts": ["other.example"],
        },
    )
    replayed = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json={**payload, "url": "https://EXAMPLE.com/article"},
    )
    conflicting = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json={**payload, "url": "https://other.example/article"},
    )

    assert original.status_code == 202
    assert replaced.status_code == 200
    assert replaced.json()["version"] == 2
    assert replayed.status_code == 202
    assert replayed.json()["job_id"] == original.json()["job_id"]
    assert conflicting.status_code == 409
    assert conflicting.json()["code"] == "idempotency_conflict"
    factory = collection_job_client.app.state.session_factory
    with factory() as session:
        assert session.execute(text("SELECT count(*) FROM jobs")).scalar_one() == 1
        assert session.execute(text("SELECT count(*) FROM outbox_messages")).scalar_one() == 1


def test_running_job_cancel_request_is_persisted_for_inflight_boundary(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    accepted = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=_payload(),
    )
    job_id = accepted.json()["job_id"]
    factory = collection_job_client.app.state.session_factory
    with factory.begin() as session:
        session.execute(
            text(
                "UPDATE jobs SET status = 'running', lease_owner = 'worker-red', "
                "lease_epoch = 1, lease_expires_at = now() + interval '1 minute', "
                "started_at = now(), updated_at = now() WHERE id = :job_id"
            ),
            {"job_id": job_id},
        )

    cancelled = collection_job_client.post(
        f"/api/jobs/{job_id}/cancel",
        headers=_csrf_headers(collection_job_client),
    )

    assert cancelled.status_code == 200, cancelled.json()
    assert cancelled.headers["cache-control"] == "no-store"
    assert cancelled.json()["status"] == "cancelling"
    assert cancelled.json()["cancellation"] == {
        "requested_at": cancelled.json()["cancellation"]["requested_at"],
        "deadline_at": cancelled.json()["cancellation"]["deadline_at"],
        "timed_out": False,
    }


def test_queued_job_cancel_is_immediate_idempotent_and_terminal_conflicts(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    accepted = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=_payload(),
    )
    location = accepted.headers["location"]
    cancel_location = f"{location}/cancel"

    first = collection_job_client.post(
        cancel_location,
        headers=_csrf_headers(collection_job_client),
    )
    repeated = collection_job_client.post(
        cancel_location,
        headers=_csrf_headers(collection_job_client),
    )

    assert first.status_code == repeated.status_code == 200
    assert first.json()["status"] == "cancelled"
    assert first.json()["completed_at"] is not None
    assert first.json()["cancellation"]["deadline_at"] is None
    assert repeated.json()["cancellation"] == first.json()["cancellation"]

    factory = collection_job_client.app.state.session_factory
    with factory.begin() as session:
        session.execute(
            text(
                "UPDATE jobs SET status = 'succeeded', completed_at = now(), "
                "cancel_requested_at = NULL, cancel_deadline_at = NULL, "
                "updated_at = now() WHERE id = :job_id"
            ),
            {"job_id": accepted.json()["job_id"]},
        )
    conflict = collection_job_client.post(
        cancel_location,
        headers=_csrf_headers(collection_job_client),
    )
    assert conflict.status_code == 409
    assert conflict.json()["code"] == "job_not_cancellable"


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


def test_cancel_route_enforces_session_csrf_and_owner_boundary(
    collection_job_client: TestClient,
) -> None:
    missing_session = collection_job_client.post(
        f"/api/jobs/{uuid4()}/cancel",
        headers={"X-HotKey-CSRF": "1"},
    )
    assert missing_session.status_code == 401
    assert missing_session.json()["code"] == "invalid_session"

    _initialize(collection_job_client)
    missing_csrf = collection_job_client.post(f"/api/jobs/{uuid4()}/cancel")
    assert missing_csrf.status_code == 403
    assert missing_csrf.json()["code"] == "csrf_invalid"

    missing_job = collection_job_client.post(
        f"/api/jobs/{uuid4()}/cancel",
        headers=_csrf_headers(collection_job_client),
    )
    assert missing_job.status_code == 404
    assert missing_job.json()["code"] == "resource_not_found"


def test_retry_rejects_non_failed_job_without_duplicate_dispatch(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    accepted = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=_payload(),
    )
    assert accepted.status_code == 202

    retried = collection_job_client.post(
        f"/api/jobs/{accepted.json()['job_id']}/retry",
        headers=_csrf_headers(collection_job_client),
    )

    assert retried.status_code == 409
    assert retried.json()["code"] == "job_not_retryable"
    factory = collection_job_client.app.state.session_factory
    with factory() as session:
        assert session.execute(text("SELECT count(*) FROM jobs")).scalar_one() == 1
        assert session.execute(text("SELECT count(*) FROM outbox_messages")).scalar_one() == 1


def test_manual_retry_reuses_failed_job_and_is_idempotent(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    accepted = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=_payload(),
    )
    job_id = accepted.json()["job_id"]
    factory = collection_job_client.app.state.session_factory
    with factory.begin() as session:
        session.execute(
            text(
                "UPDATE jobs SET status = 'failed', completed_at = now(), retry_count = 0, "
                "last_error_code = 'connection_expired', "
                "last_error_category = 'authentication_required', last_error_at = now(), "
                "next_action = '修复连接后重试', manual_retry_allowed = true, "
                "updated_at = now() WHERE id = :job_id"
            ),
            {"job_id": job_id},
        )

    first = collection_job_client.post(
        f"/api/jobs/{job_id}/retry",
        headers=_csrf_headers(collection_job_client),
    )
    repeated = collection_job_client.post(
        f"/api/jobs/{job_id}/retry",
        headers=_csrf_headers(collection_job_client),
    )

    assert first.status_code == repeated.status_code == 202
    assert first.headers["location"] == f"/api/jobs/{job_id}"
    assert first.headers["cache-control"] == "no-store"
    assert first.json()["id"] == repeated.json()["id"] == job_id
    assert first.json()["status"] == repeated.json()["status"] == "queued"
    assert first.json()["retry_count"] == repeated.json()["retry_count"] == 1
    assert first.json()["failure"] == {
        "error_code": "connection_expired",
        "category": "authentication_required",
        "occurred_at": first.json()["failure"]["occurred_at"],
        "next_action": "修复连接后重试",
        "manual_retry_allowed": True,
    }
    with factory() as session:
        rows = session.execute(
            text(
                "SELECT dispatch_sequence, event_type FROM outbox_messages "
                "ORDER BY dispatch_sequence"
            )
        ).all()
    assert rows == [(1, "job.accepted.v2"), (2, "job.retry_scheduled.v1")]


def test_retry_route_enforces_session_csrf_and_missing_resource_boundary(
    collection_job_client: TestClient,
) -> None:
    missing_session = collection_job_client.post(
        f"/api/jobs/{uuid4()}/retry",
        headers={"X-HotKey-CSRF": "1"},
    )
    assert missing_session.status_code == 401
    assert missing_session.json()["code"] == "invalid_session"

    _initialize(collection_job_client)
    missing_csrf = collection_job_client.post(f"/api/jobs/{uuid4()}/retry")
    assert missing_csrf.status_code == 403
    assert missing_csrf.json()["code"] == "csrf_invalid"

    missing_job = collection_job_client.post(
        f"/api/jobs/{uuid4()}/retry",
        headers=_csrf_headers(collection_job_client),
    )
    assert missing_job.status_code == 404
    assert missing_job.json()["code"] == "resource_not_found"


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
    cancel_operation = schema["paths"]["/api/jobs/{job_id}/cancel"]["post"]
    retry_operation = schema["paths"]["/api/jobs/{job_id}/retry"]["post"]

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
    assert cancel_operation["operationId"] == "cancelCollectionJob"
    assert cancel_operation["responses"]["200"]["content"]["application/json"]["schema"] == {
        "$ref": "#/components/schemas/JobStatusView"
    }
    assert cancel_operation["security"] == [{"SessionCookie": []}]
    assert retry_operation["operationId"] == "retryCollectionJob"
    assert "200" not in retry_operation["responses"]
    assert retry_operation["responses"]["202"]["content"]["application/json"]["schema"] == {
        "$ref": "#/components/schemas/JobStatusView"
    }
    assert retry_operation["security"] == [{"SessionCookie": []}]
    for status_code in ("401", "403", "409", "422", "500"):
        assert create_operation["responses"][status_code]["content"]["application/json"][
            "schema"
        ] == {"$ref": "#/components/schemas/ErrorView"}
