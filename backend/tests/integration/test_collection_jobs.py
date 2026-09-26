from __future__ import annotations

import os
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, text
from sqlalchemy.orm import Session

from core.config import Settings
from jobs.schemas import JobAcceptanceInput
from jobs.services import JobService
from main import create_app

_BOOTSTRAP_TOKEN = "bootstrap-token-used-only-by-the-isolated-test"
_PASSWORD = "correct horse battery staple"
_TRUNCATE = (
    "TRUNCATE content_version_relations, content_visibility_observations, "
    "content_observations, content_versions, "
    "content_discoveries, content_threads, content_records, "
    "source_capability_evidence, source_connection_versions, "
    "source_connections, provenance_manifest_inputs, provenance_manifests, "
    "evidence_cleanup_targets, evidence_deletions, "
    "evidence_resources, evidence_retention_policies, source_access_policies, "
    "resource_budget_reservations, resource_budget_windows, resource_budget_policies, "
    "resource_usage_attempts, resource_component_policies, job_stage_attempts, "
    "processed_messages, job_attempts, "
    "ai_calls, knowledge_exports, notification_deliveries, "
    "notification_targets, content_annotations, reports, "
    "monitor_schedules, outbox_messages, coverage_windows, "
    "jobs, followed_account_aliases, followed_accounts, "
    "monitor_topic_versions, monitor_topics, "
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


def _payload(
    *,
    operation_id: UUID | None = None,
    window: int = 7,
    configuration_version: int = 3,
) -> dict[str, object]:
    return {
        "operation_id": str(operation_id or uuid4()),
        "kind": "monitor.collect",
        "observation": {
            "configuration_ref": "monitor-config-1",
            "configuration_version": configuration_version,
            "source_key": "x",
            "source_capability": "search",
        },
        "scheduled_for_at": None,
        "scope": {"source_id": "account-1", "window": window},
    }


def _csrf_headers(client: TestClient) -> dict[str, str]:
    return {"X-HotKey-CSRF": client.cookies["hotkey_csrf"]}


def _accept_internal_job(client: TestClient, payload: dict[str, object]) -> UUID:
    factory = client.app.state.session_factory
    with factory() as session:
        owner_id = session.execute(
            text("SELECT id FROM identity_users WHERE username = 'owner'")
        ).scalar_one()
        job = JobService(session).accept(
            owner_id=owner_id,
            command=JobAcceptanceInput.model_validate(payload, strict=False),
        )
    return job.id


def _set_job_facts(
    session: Session,
    *,
    job_id: UUID,
    status: str,
    started_at: datetime,
    completed_at: datetime | None,
    outcome: str,
    delayed_at: datetime | None = None,
) -> None:
    updated_at = delayed_at or completed_at
    assert updated_at is not None
    session.execute(
        text(
            "INSERT INTO job_attempts "
            "(id, job_id, lease_epoch, worker_id, started_at, lease_expires_at, "
            "finished_at, outcome) VALUES (:id, :job_id, 1, 'freshness-test', "
            ":started_at, :lease_expires_at, :finished_at, :outcome)"
        ),
        {
            "id": uuid4(),
            "job_id": job_id,
            "started_at": started_at,
            "lease_expires_at": started_at + timedelta(minutes=10),
            "finished_at": updated_at,
            "outcome": outcome,
        },
    )
    session.execute(
        text(
            "UPDATE jobs SET created_at = :created_at, status = :status, "
            "started_at = :started_at, completed_at = :completed_at, updated_at = :updated_at, "
            "defer_reason = :defer_reason, next_run_at = :next_run_at, retry_count = :retry_count, "
            "last_error_code = :error_code, last_error_category = :error_category, "
            "last_error_at = :error_at, next_action = :next_action "
            "WHERE id = :job_id"
        ),
        {
            "job_id": job_id,
            "created_at": started_at - timedelta(minutes=5),
            "status": status,
            "started_at": started_at,
            "completed_at": completed_at,
            "updated_at": updated_at,
            "defer_reason": "rate_limited" if delayed_at is not None else None,
            "next_run_at": delayed_at + timedelta(minutes=10) if delayed_at else None,
            "retry_count": 1 if delayed_at is not None else 0,
            "error_code": "collector_budget_exhausted" if delayed_at is not None else None,
            "error_category": "rate_limited" if delayed_at is not None else None,
            "error_at": delayed_at,
            "next_action": "等待预算窗口恢复" if delayed_at is not None else None,
        },
    )


def test_internal_job_status_is_readable_after_refresh(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    payload = _payload()
    job_id = _accept_internal_job(collection_job_client, payload)

    refreshed = collection_job_client.get(f"/api/jobs/{job_id}")
    body = refreshed.json()
    freshness = body["source_freshness"]

    assert refreshed.status_code == 200
    assert refreshed.headers["cache-control"] == "no-store"
    assert body == {
        "id": str(job_id),
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
        "created_at": body["created_at"],
        "source_freshness": freshness,
        "coverage_windows": [],
    }
    assert freshness == {
        "last_attempt_at": None,
        "last_success_at": None,
        "delay_reason": "internal_queue",
        "delay_since_at": body["created_at"],
        "delay_duration_us": freshness["delay_duration_us"],
    }
    assert isinstance(freshness["delay_duration_us"], int)
    assert freshness["delay_duration_us"] >= 0
    assert "owner_id" not in body
    assert "scope" not in body


def test_job_status_reports_last_attempt_full_success_and_budget_delay(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    success_job_id = _accept_internal_job(collection_job_client, _payload())
    partial_job_id = _accept_internal_job(collection_job_client, _payload())
    delayed_job_id = _accept_internal_job(collection_job_client, _payload())
    other_version_job_id = _accept_internal_job(
        collection_job_client,
        _payload(configuration_version=4),
    )

    factory = collection_job_client.app.state.session_factory
    now = datetime.now(UTC)
    last_success_at = now - timedelta(hours=1)
    partial_completed_at = now - timedelta(minutes=10)
    last_attempt_at = now - timedelta(minutes=5)
    delayed_at = now - timedelta(minutes=2)
    other_version_success_at = now - timedelta(minutes=1)
    jobs = (
        (
            success_job_id,
            "succeeded",
            last_success_at - timedelta(minutes=2),
            last_success_at,
            None,
        ),
        (
            partial_job_id,
            "partially_succeeded",
            partial_completed_at - timedelta(minutes=2),
            partial_completed_at,
            None,
        ),
        (delayed_job_id, "queued", last_attempt_at, None, delayed_at),
        (
            other_version_job_id,
            "succeeded",
            other_version_success_at - timedelta(minutes=1),
            other_version_success_at,
            None,
        ),
    )
    with factory() as session, session.begin():
        for job_id, status, started_at, job_completed_at, delay_at in jobs:
            _set_job_facts(
                session,
                job_id=job_id,
                status=status,
                started_at=started_at,
                completed_at=job_completed_at,
                outcome="delayed" if delay_at is not None else "succeeded",
                delayed_at=delay_at,
            )

    response = collection_job_client.get(f"/api/jobs/{delayed_job_id}")

    assert response.status_code == 200, response.json()
    freshness = response.json()["source_freshness"]
    assert datetime.fromisoformat(freshness["last_attempt_at"].replace("Z", "+00:00")) == (
        last_attempt_at
    )
    assert datetime.fromisoformat(freshness["last_success_at"].replace("Z", "+00:00")) == (
        last_success_at
    )
    assert freshness["delay_reason"] == "budget_exhausted"
    assert datetime.fromisoformat(freshness["delay_since_at"].replace("Z", "+00:00")) == (
        delayed_at
    )
    assert freshness["delay_duration_us"] >= 0


def test_job_history_uses_owner_scoped_stable_cursor_and_safe_summary(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)
    job_ids: list[str] = []
    for window in (1, 2, 3):
        job_ids.append(str(_accept_internal_job(collection_job_client, _payload(window=window))))

    created_at = datetime(2026, 9, 23, 8, tzinfo=UTC)
    factory = collection_job_client.app.state.session_factory
    with factory() as session, session.begin():
        for job_id in job_ids:
            session.execute(
                text(
                    "UPDATE jobs SET created_at = :created_at, updated_at = :created_at "
                    "WHERE id = :job_id"
                ),
                {"created_at": created_at, "job_id": job_id},
            )

    history = collection_job_client.get("/api/jobs", params={"limit": 2})

    assert history.status_code == 200, history.json()
    assert history.headers["cache-control"] == "no-store"
    expected = sorted(
        (collection_job_client.get(f"/api/jobs/{job_id}").json() for job_id in job_ids),
        key=lambda job: (job["created_at"], job["id"]),
        reverse=True,
    )
    first_page = history.json()
    assert [item["id"] for item in first_page["items"]] == [item["id"] for item in expected[:2]]
    assert first_page["next_cursor"] == expected[1]["id"]
    assert set(first_page["items"][0]) == {
        "id",
        "kind",
        "source_key",
        "source_capability",
        "status",
        "requests_sent",
        "items_saved",
        "created_at",
        "started_at",
        "completed_at",
        "next_run_at",
    }
    assert "owner_id" not in first_page["items"][0]
    assert "scope" not in first_page["items"][0]
    assert "request_fingerprint" not in first_page["items"][0]

    next_page = collection_job_client.get(
        "/api/jobs",
        params={"cursor": first_page["next_cursor"], "limit": 2},
    )
    assert next_page.status_code == 200, next_page.json()
    assert [item["id"] for item in next_page.json()["items"]] == [
        item["id"] for item in expected[2:]
    ]
    assert next_page.json()["next_cursor"] is None
    assert collection_job_client.get("/api/jobs", params={"limit": 0}).status_code == 422
    assert (
        collection_job_client.get(
            "/api/jobs",
            params={"cursor": str(uuid4())},
        ).status_code
        == 404
    )


def test_job_history_requires_an_authenticated_owner(
    collection_job_client: TestClient,
) -> None:
    response = collection_job_client.get("/api/jobs")

    assert response.status_code == 401


def test_continuous_failure_issue_endpoint_is_authenticated_and_redacted(
    collection_job_client: TestClient,
) -> None:
    assert collection_job_client.get("/api/jobs/issues").status_code == 401
    _initialize(collection_job_client)

    job_ids: list[str] = []
    for window in (1, 2, 3):
        payload = _payload(window=window)
        payload["observation"] = {
            **payload["observation"],
            "configuration_version": window,
        }
        job_ids.append(str(_accept_internal_job(collection_job_client, payload)))

    now = datetime.now(UTC)
    factory = collection_job_client.app.state.session_factory
    with factory() as session, session.begin():
        for index, job_id in enumerate(job_ids, start=1):
            completed_at = now + timedelta(minutes=index)
            session.execute(
                text(
                    "UPDATE jobs SET status = 'failed', started_at = :started_at, "
                    "completed_at = :completed_at, updated_at = :completed_at, "
                    "last_error_code = 'source.timeout', "
                    "last_error_category = 'transient', last_error_at = :completed_at, "
                    "next_action = '检查来源连接' WHERE id = :job_id"
                ),
                {
                    "started_at": completed_at - timedelta(seconds=1),
                    "completed_at": completed_at,
                    "job_id": job_id,
                },
            )

    response = collection_job_client.get("/api/jobs/issues")

    assert response.status_code == 200, response.json()
    assert response.headers["cache-control"] == "no-store"
    issues = response.json()
    assert len(issues) == 1
    assert set(issues[0]) == {
        "source_key",
        "source_capability",
        "configuration_ref",
        "configuration_version",
        "latest_failed_job_id",
        "failure",
        "consecutive_failure_threshold",
    }
    assert issues[0]["source_key"] == "x"
    assert issues[0]["source_capability"] == "search"
    assert issues[0]["configuration_ref"] == "monitor-config-1"
    assert issues[0]["latest_failed_job_id"] == job_ids[-1]
    assert issues[0]["configuration_version"] == 3
    assert issues[0]["consecutive_failure_threshold"] == 3
    assert issues[0]["failure"]["error_code"] == "source.timeout"
    assert issues[0]["failure"]["next_action"] == "检查来源连接"
    assert "scope" not in str(issues)
    assert "source_id" not in str(issues)
    assert "owner_id" not in str(issues)


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
    job_id = str(_accept_internal_job(collection_job_client, _payload()))
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
    job_id = _accept_internal_job(collection_job_client, _payload())
    location = f"/api/jobs/{job_id}"
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
            {"job_id": job_id},
        )
    conflict = collection_job_client.post(
        cancel_location,
        headers=_csrf_headers(collection_job_client),
    )
    assert conflict.status_code == 409
    assert conflict.json()["code"] == "job_not_cancellable"


def test_job_routes_enforce_session_csrf_and_missing_resource_boundaries(
    collection_job_client: TestClient,
) -> None:
    payload = {
        "operation_id": str(uuid4()),
        "kind": "webpage.collect",
        "url": "https://example.com/article",
    }
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
    job_id = _accept_internal_job(collection_job_client, _payload())

    retried = collection_job_client.post(
        f"/api/jobs/{job_id}/retry",
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
    job_id = str(_accept_internal_job(collection_job_client, _payload()))
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


def test_public_submission_rejects_job_kind_without_worker_handler(
    collection_job_client: TestClient,
) -> None:
    _initialize(collection_job_client)

    response = collection_job_client.post(
        "/api/jobs",
        headers=_csrf_headers(collection_job_client),
        json=_payload(),
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
    assert create_operation["requestBody"]["content"]["application/json"]["schema"] == {
        "$ref": "#/components/schemas/WebPageCollectionJobInput"
    }
    assert "CollectionJobInput" not in schema["components"]["schemas"]
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
