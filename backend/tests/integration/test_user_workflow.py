import os

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import func, select

from api.dependencies import collection_service
from audit.models import Audit
from collection.services import CollectionService
from core.clock import utcnow
from core.config import Settings
from identity.services import IdentityService
from main import create_app
from monitors.models import MonitorVersion
from monitors.services import MonitorService
from sources.services import SourceService

pytestmark = pytest.mark.integration


@pytest.fixture
def client(database):
    IdentityService(database).bootstrap("learner", "Test-password-123!")
    settings = Settings(
        database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
        broker_url="amqp://u:p@localhost/test",
        allowed_origins=["http://testserver"],
    )
    with TestClient(create_app(settings)) as client:
        client.headers["Origin"] = "http://testserver"
        yield client


def login(client):
    result = client.post(
        "/api/v1/session", json={"username": "learner", "password": "Test-password-123!"}
    )
    assert result.status_code == 200
    assert "HttpOnly" in result.headers.get_list("set-cookie")[0]
    client.headers["X-CSRF-Token"] = client.cookies["hk_csrf"]


def test_auth_csrf_revocation_and_no_password_echo(client):
    assert client.get("/api/v1/monitors").status_code == 401
    assert client.get("/api/v1/contents").status_code == 401
    assert client.get("/api/v1/collection-runs").status_code == 401
    result = client.post("/api/v1/session", json={"username": "learner", "password": "secret"})
    assert result.status_code == 422 and "secret" not in result.text
    login(client)
    assert client.get("/api/v1/contents").json() == {"items": [], "next_cursor": None}
    assert client.get("/api/v1/collection-runs").json() == {
        "items": [],
        "next_cursor": None,
    }
    assert client.get("/api/v1/contents?cursor=bad").status_code == 422
    old_token = client.cookies["hk_session"]
    client.headers.pop("X-CSRF-Token")
    assert client.delete("/api/v1/session").status_code == 403
    client.headers["X-CSRF-Token"] = client.cookies["hk_csrf"]
    assert client.delete("/api/v1/session").status_code == 204
    client.cookies.set("hk_session", old_token)
    assert client.get("/api/v1/session").status_code == 401


def test_origin_and_request_size_are_bounded(client):
    assert (
        client.post("/api/v1/session", headers={"Origin": "https://evil.invalid"}).status_code
        == 403
    )
    result = client.post("/api/v1/session", content=b"a" * 65537)
    assert result.status_code == 413
    assert result.json()["request_id"] == result.headers["x-request-id"]


def test_monitor_version_conflict_and_no_fake_collection(client, database):
    login(client)
    body = {
        "title": "AI 观察",
        "query_spec": {
            "include_any": ["AI", " AI ", "人工智能"],
            "include_all": ["监管"],
            "exclude": [],
            "aliases": ["生成式AI"],
        },
        "source_ids": ["weibo", "bilibili"],
        "schedule": {"interval_minutes": 60},
        "budget": {"daily_requests": 24, "content_purchase_cost": 0},
    }
    result = client.post("/api/v1/monitors", json=body)
    assert result.status_code == 201
    monitor = result.json()
    assert monitor["query_spec"]["include_any"] == ["AI", "人工智能"]
    assert monitor["state"] == "draft" and monitor["current_version"] == 1
    path = "/api/v1/monitors/" + monitor["id"]
    update = dict(body, title="更新", expected_version=1)
    assert client.patch(path, json=update).json()["current_version"] == 2
    assert client.patch(path, json=update).status_code == 409
    assert client.get("/api/v1/monitors?limit=101").status_code == 422
    sources = client.get("/api/v1/sources").json()
    assert all(
        operation["pipeline"] == "not_connected" and not operation["eligible_for_collection"]
        for source in sources
        for operation in source["operations"]
    )
    assert all("pipeline" not in source for source in sources)
    bilibili = next(source for source in sources if source["id"] == "bilibili")
    assert {operation["operation"] for operation in bilibili["operations"]} == {
        "search_posts",
        "fetch_post",
        "list_comments",
        "list_replies",
    }
    with database() as session:
        versions = list(
            session.scalars(
                select(MonitorVersion)
                .where(MonitorVersion.monitor_id == monitor["id"])
                .order_by(MonitorVersion.version)
            )
        )
        assert [version.title for version in versions] == ["AI 观察", "更新"]
        assert (
            session.scalar(
                select(func.count()).select_from(Audit).where(Audit.action == "monitor_updated")
            )
            == 1
        )


def test_exact_search_admission_is_shared_by_api_and_monitor_activation(database):
    IdentityService(database).bootstrap("learner", "Test-password-123!")
    settings = Settings(
        database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
        broker_url="amqp://u:p@localhost/test",
        allowed_origins=["http://testserver"],
        s3_endpoint="minio.internal:9000",
        s3_access_key="access",
        s3_secret_key="secret",
        s3_bucket="hotkey-evidence-test",
        source_rights_allowed=["bilibili.search_posts", "bilibili.fetch_post"],
        source_pipelines_connected=["bilibili.search_posts", "bilibili.fetch_post"],
    )
    with TestClient(create_app(settings)) as admitted:
        admitted.headers["Origin"] = "http://testserver"
        login(admitted)
        source = next(
            item for item in admitted.get("/api/v1/sources").json() if item["id"] == "bilibili"
        )
        assert "pipeline" not in source and "eligible_for_collection" not in source
        operations = {item["operation"]: item for item in source["operations"]}
        assert operations["search_posts"]["rights"] == "allowed"
        assert operations["search_posts"]["pipeline"] == "connected"
        assert operations["search_posts"]["eligible_for_collection"] is True
        assert operations["list_comments"]["pipeline"] == "not_connected"
        assert operations["list_comments"]["eligible_for_collection"] is False

        created = admitted.post(
            "/api/v1/monitors",
            json={
                "title": "精确准入",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "schedule": {"interval_minutes": 60, "retention_days": 7},
                "budget": {"daily_requests": 48, "content_purchase_cost": 0},
            },
        ).json()
        activated = admitted.post(
            f"/api/v1/monitors/{created['id']}/activate",
            json={"expected_version": created["current_version"]},
        )
        assert activated.status_code == 200
        assert activated.json()["state"] == "active"


def test_diagnostic_idempotency_and_cancellation(client):
    login(client)
    args = {"json": {"kind": "verify_pipeline"}, "headers": {"Idempotency-Key": "test-diagnostic"}}
    a = client.post("/api/v1/jobs", **args)
    b = client.post("/api/v1/jobs", **args)
    assert a.status_code == 201 and a.json()["id"] == b.json()["id"]
    assert client.post("/api/v1/jobs", json={"kind": "collect"}).status_code == 422
    assert client.post("/api/v1/jobs/" + a.json()["id"] + "/cancel").json()["status"] == "cancelled"


def test_login_throttle_persists_failures(client):
    for _ in range(5):
        assert (
            client.post(
                "/api/v1/session", json={"username": "learner", "password": "Wrong-password-123!"}
            ).status_code
            == 401
        )
    response = client.post(
        "/api/v1/session", json={"username": "learner", "password": "Test-password-123!"}
    )
    assert response.status_code == 429 and response.headers["Retry-After"] == "900"


def test_expired_session_and_pagination(client, database):
    from datetime import timedelta

    from sqlalchemy import update

    from identity.models import LoginSession

    login(client)
    for title in ["one", "two", "three"]:
        assert (
            client.post(
                "/api/v1/monitors",
                json={
                    "title": title,
                    "query_spec": {"include_any": ["AI"]},
                    "source_ids": ["bilibili"],
                },
            ).status_code
            == 201
        )
    first = client.get("/api/v1/monitors?limit=2").json()
    second = client.get(
        "/api/v1/monitors", params={"limit": 2, "cursor": first["next_cursor"]}
    ).json()
    assert len({m["id"] for m in first["items"] + second["items"]}) == 3
    assert second["next_cursor"] is None
    with database.begin() as session:
        session.execute(update(LoginSession).values(expires_at=utcnow() - timedelta(seconds=1)))
    assert client.get("/api/v1/session").status_code == 401


def test_unexpected_error_is_sanitized(client, monkeypatch):
    login(client)

    def broken(*_args, **_kwargs):
        raise RuntimeError("private-credential-must-not-escape")

    monkeypatch.setattr(MonitorService, "monitors", broken)
    result = client.get("/api/v1/monitors")
    assert result.status_code == 500
    assert result.json()["code"] == "internal_error"
    assert "private-credential" not in result.text


def test_query_preview_auth_window_and_no_fake_connection(client):
    path = "/api/v1/sources/query-preview"
    query = {
        "query_spec": {"include_any": ["科学"], "exclude": ["广告"]},
        "source_ids": ["bilibili", "xiaohongshu"],
        "since": "2026-09-01T08:00:00+08:00",
        "until": "2026-09-08T00:00:00Z",
    }
    assert client.post(path, json=query).status_code == 401
    login(client)
    response = client.post(path, json=query)
    assert response.status_code == 200
    data = response.json()
    assert data["since"] == "2026-09-01T00:00:00Z"
    assert data["sources"][0]["queries"] == ["科学"]
    assert data["sources"][1]["queries"] == []
    assert data["network_accessed"] is False
    assert client.post(path, json=dict(query, until=query["since"])).status_code == 422
    client.headers.pop("X-CSRF-Token")
    assert client.post(path, json=query).status_code == 403


def test_monitor_activation_is_admission_gated_and_pause_is_explicit(client, database, monkeypatch):
    login(client)
    created = client.post(
        "/api/v1/monitors",
        json={
            "title": "受控启停",
            "query_spec": {"include_any": ["AI"]},
            "source_ids": ["bilibili"],
            "budget": {"daily_requests": 47, "content_purchase_cost": 0},
        },
    ).json()
    path = f"/api/v1/monitors/{created['id']}"
    state = {"expected_version": created["current_version"]}
    refused = client.post(path + "/activate", json=state)
    assert refused.status_code == 409
    assert refused.json()["code"] == "source_not_eligible"

    monkeypatch.setattr(
        SourceService, "activation_issues", lambda self, source_ids, operation="search_posts": []
    )
    budget_refused = client.post(path + "/activate", json=state)
    assert budget_refused.status_code == 409
    assert budget_refused.json()["code"] == "monitor_budget_insufficient"
    updated = client.patch(
        path,
        json={
            "title": "受控启停",
            "query_spec": {"include_any": ["AI"]},
            "source_ids": ["bilibili"],
            "budget": {"daily_requests": 48, "content_purchase_cost": 0},
            "expected_version": created["current_version"],
        },
    ).json()
    state = {"expected_version": updated["current_version"]}
    activated = client.post(path + "/activate", json=state)
    assert activated.status_code == 200 and activated.json()["state"] == "active"
    missing_store = client.post(
        path + "/runs",
        headers={"Idempotency-Key": "missing-store"},
        json={
            "expected_version": updated["current_version"],
            "source": "bilibili",
            "request_value": "AI",
            "since": "2026-09-14T00:00:00Z",
            "until": "2026-09-15T00:00:00Z",
            "policy_version": "user-confirmed-policy-v1",
            "retention_days": 7,
        },
    )
    assert missing_store.status_code == 409
    assert missing_store.json()["code"] == "evidence_store_not_configured"
    client.app.dependency_overrides[collection_service] = lambda: CollectionService(
        database, evidence_configured=True
    )
    accepted = client.post(
        path + "/runs",
        headers={"Idempotency-Key": "accepted-run"},
        json={
            "expected_version": updated["current_version"],
            "source": "bilibili",
            "request_value": "AI",
            "since": "2026-09-14T00:00:00Z",
            "until": "2026-09-15T00:00:00Z",
            "policy_version": "user-confirmed-policy-v1",
            "retention_days": 7,
        },
    )
    assert accepted.status_code == 201
    run = client.get(f"/api/v1/collection-runs/{accepted.json()['id']}")
    assert run.status_code == 200 and run.json()["job_id"] == accepted.json()["job_id"]
    client.app.dependency_overrides.pop(collection_service)
    assert (
        client.patch(
            path,
            json={
                "title": "不能覆盖运行快照",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "expected_version": updated["current_version"],
            },
        ).json()["code"]
        == "monitor_not_editable"
    )
    paused = client.post(path + "/pause", json=state)
    assert paused.status_code == 200 and paused.json()["state"] == "paused"
    assert client.post(path + "/pause", json=state).json()["code"] == "monitor_state_conflict"
    revised = client.patch(
        path,
        json={
            "title": "暂停后修订",
            "query_spec": {"include_any": ["AI", "智能体"]},
            "source_ids": ["bilibili"],
            "budget": {"daily_requests": 96, "content_purchase_cost": 0},
            "expected_version": updated["current_version"],
        },
    ).json()
    assert revised["state"] == "paused" and revised["current_version"] == 3
    reactivated = client.post(
        path + "/activate", json={"expected_version": revised["current_version"]}
    )
    assert reactivated.status_code == 200 and reactivated.json()["state"] == "active"
