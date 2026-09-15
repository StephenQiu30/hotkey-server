import os

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import func, select

from audit.models import Audit
from core.clock import utcnow
from core.config import Settings
from identity.services import IdentityService
from main import create_app
from monitors.services import MonitorService

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
    result = client.post("/api/v1/session", json={"username": "learner", "password": "secret"})
    assert result.status_code == 422 and "secret" not in result.text
    login(client)
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
        "keywords": ["AI", " AI ", "人工智能"],
        "sources": ["weibo", "bilibili"],
    }
    result = client.post("/api/v1/monitors", json=body)
    assert result.status_code == 201
    monitor = result.json()
    assert monitor["keywords"] == ["AI", "人工智能"] and monitor["status"] == "draft"
    path = "/api/v1/monitors/" + monitor["id"]
    update = dict(body, title="更新", version=1)
    assert client.patch(path, json=update).json()["version"] == 2
    assert client.patch(path, json=update).status_code == 409
    assert client.get("/api/v1/monitors?limit=101").status_code == 422
    sources = client.get("/api/v1/sources").json()
    assert all(source["pipeline"] == "not_connected" for source in sources)
    assert all(not source["eligible_for_collection"] for source in sources)
    bilibili = next(source for source in sources if source["id"] == "bilibili")
    assert {operation["operation"] for operation in bilibili["operations"]} == {
        "search_posts",
        "fetch_post",
        "list_comments",
        "list_replies",
    }
    with database() as session:
        assert (
            session.scalar(
                select(func.count()).select_from(Audit).where(Audit.action == "monitor_updated")
            )
            == 1
        )


def test_diagnostic_idempotency_and_cancellation(client):
    login(client)
    args = {"json": {"kind": "verify_pipeline"}, "headers": {"Idempotency-Key": "test-diagnostic"}}
    a = client.post("/api/v1/jobs", **args)
    b = client.post("/api/v1/jobs", **args)
    assert a.status_code == 202 and a.json()["id"] == b.json()["id"]
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
                json={"title": title, "keywords": ["AI"], "sources": ["bilibili"]},
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
    path = "/api/v1/sources/bluesky/query-preview"
    query = {
        "keyword": "科学",
        "since": "2026-09-01T08:00:00+08:00",
        "until": "2026-09-08T00:00:00Z",
    }
    assert client.post(path, json=query).status_code == 401
    login(client)
    response = client.post(path, json=query)
    assert response.status_code == 200
    data = response.json()
    assert data["query"] == "科学" and data["since"] == "2026-09-01T00:00:00Z"
    assert data["coverage"] == "unknown" and data["pipeline_connected"] is False
    assert client.post(path, json=dict(query, until=query["since"])).status_code == 422
    client.headers.pop("X-CSRF-Token")
    assert client.post(path, json=query).status_code == 403
