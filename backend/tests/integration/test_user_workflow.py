import os
from datetime import timedelta

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import func, select

from api.dependencies import collection_service
from audit.models import Audit
from collection.models import CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from core.clock import utcnow
from core.config import Settings
from identity.services import IdentityService
from main import create_app
from monitors.models import MonitorVersion
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


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
        "/api/session", json={"username": "learner", "password": "Test-password-123!"}
    )
    assert result.status_code == 200
    assert "HttpOnly" in result.headers.get_list("set-cookie")[0]
    client.headers["X-CSRF-Token"] = client.cookies["hk_csrf"]


def test_auth_csrf_revocation_and_no_password_echo(client):
    assert client.get("/api/monitors").status_code == 401
    assert client.get("/api/contents").status_code == 401
    assert client.get("/api/events").status_code == 401
    assert client.get("/api/notifications").status_code == 401
    assert client.get("/api/collection-runs").status_code == 401
    result = client.post("/api/session", json={"username": "learner", "password": "secret"})
    assert result.status_code == 422 and "secret" not in result.text
    login(client)
    assert client.get("/api/contents").json() == {"items": [], "next_cursor": None}
    assert client.get("/api/events").json() == {"items": [], "next_cursor": None}
    assert client.get("/api/notifications").json() == {
        "items": [],
        "next_cursor": None,
        "unread_count": 0,
    }
    assert client.get("/api/collection-runs").json() == {
        "items": [],
        "next_cursor": None,
    }
    assert client.get("/api/contents?cursor=bad").status_code == 422
    old_token = client.cookies["hk_session"]
    client.headers.pop("X-CSRF-Token")
    assert client.delete("/api/session").status_code == 403
    client.headers["X-CSRF-Token"] = client.cookies["hk_csrf"]
    assert client.delete("/api/session").status_code == 204
    client.cookies.set("hk_session", old_token)
    assert client.get("/api/session").status_code == 401


def test_origin_and_request_size_are_bounded(client):
    assert (
        client.post("/api/session", headers={"Origin": "https://evil.invalid"}).status_code == 403
    )
    result = client.post("/api/session", content=b"a" * 65537)
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
    result = client.post("/api/monitors", json=body)
    assert result.status_code == 201
    monitor = result.json()
    assert monitor["query_spec"]["include_any"] == ["AI", "人工智能"]
    assert monitor["state"] == "draft" and monitor["current_version"] == 1
    path = "/api/monitors/" + monitor["id"]
    update = dict(body, title="更新", expected_version=1)
    assert client.patch(path, json=update).json()["current_version"] == 2
    assert client.patch(path, json=update).status_code == 409
    assert client.get("/api/monitors?limit=101").status_code == 422
    sources = client.get("/api/sources").json()
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
        source_rights_allowed=[
            "bilibili.search_posts",
            "bilibili.fetch_post",
            "bilibili.list_comments",
            "bilibili.list_replies",
        ],
        source_pipelines_connected=[
            "bilibili.search_posts",
            "bilibili.fetch_post",
            "bilibili.list_comments",
            "bilibili.list_replies",
        ],
    )
    with TestClient(create_app(settings)) as admitted:
        admitted.headers["Origin"] = "http://testserver"
        login(admitted)
        source = next(
            item for item in admitted.get("/api/sources").json() if item["id"] == "bilibili"
        )
        assert "pipeline" not in source and "eligible_for_collection" not in source
        operations = {item["operation"]: item for item in source["operations"]}
        assert operations["search_posts"]["rights"] == "allowed"
        assert operations["search_posts"]["pipeline"] == "connected"
        assert operations["search_posts"]["eligible_for_collection"] is True
        assert operations["list_comments"]["pipeline"] == "connected"
        assert operations["list_comments"]["eligible_for_collection"] is True
        assert operations["list_replies"]["pipeline"] == "connected"
        assert operations["list_replies"]["eligible_for_collection"] is True

        created = admitted.post(
            "/api/monitors",
            json={
                "title": "精确准入",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "schedule": {"interval_minutes": 60, "retention_days": 7},
                "budget": {"daily_requests": 192, "content_purchase_cost": 0},
            },
        ).json()
        activated = admitted.post(
            f"/api/monitors/{created['id']}/activate",
            json={"expected_version": created["current_version"]},
        )
        assert activated.status_code == 200
        assert activated.json()["state"] == "active"


def test_source_catalog_projects_latest_runtime_health(client, database):
    login(client)
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    draft = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "来源健康",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "budget": {"daily_requests": 1000, "content_purchase_cost": 0},
            }
        )
    )
    active = monitors.change_state(
        draft.id,
        MonitorStateChange(expected_version=draft.current_version),
        "active",
    )

    def create(key: str):
        return CollectionService(database, sources, evidence_configured=True).create_run(
            CollectionRunInput(
                monitor_id=active.id,
                expected_version=active.current_version,
                source="bilibili",
                request_value="AI",
                since="2026-09-14T00:00:00Z",
                until="2026-09-15T00:00:00Z",
                idempotency_key=key,
                policy_version="synthetic-policy-v1",
                retention_days=7,
                ingestion_mode="live",
            )
        )

    success = create("runtime-health-success")
    failure = create("runtime-health-failure")
    now = utcnow()
    with database.begin() as session:
        succeeded = session.get(CollectionRun, success.id)
        failed = session.get(CollectionRun, failure.id)
        assert succeeded is not None and failed is not None
        succeeded.state = "completed"
        succeeded.outcome = "ok"
        succeeded.completed_at = now - timedelta(minutes=2)
        failed.state = "failed"
        failed.outcome = "failed"
        failed.stop_reason = "access_denied"
        failed.completed_at = now - timedelta(minutes=1)

    catalog = client.get("/api/sources")
    assert catalog.status_code == 200
    bilibili = next(source for source in catalog.json() if source["id"] == "bilibili")
    operations = {item["operation"]: item for item in bilibili["operations"]}
    assert operations["search_posts"]["runtime"] == {
        "status": "degraded",
        "last_success_at": (now - timedelta(minutes=2)).isoformat().replace("+00:00", "Z"),
        "last_failure_at": (now - timedelta(minutes=1)).isoformat().replace("+00:00", "Z"),
        "last_failure_code": "access_denied",
        "recovery_action": "refresh_authorization",
    }
    assert operations["list_comments"]["runtime"]["status"] == "unobserved"

    recovered = create("runtime-health-recovered")
    with database.begin() as session:
        row = session.get(CollectionRun, recovered.id)
        assert row is not None
        row.state = "completed"
        row.outcome = "empty"
        row.completed_at = now

    catalog = client.get("/api/sources").json()
    bilibili = next(source for source in catalog if source["id"] == "bilibili")
    runtime = next(
        item["runtime"] for item in bilibili["operations"] if item["operation"] == "search_posts"
    )
    assert runtime["status"] == "healthy"
    assert runtime["last_success_at"] == now.isoformat().replace("+00:00", "Z")
    assert runtime["last_failure_code"] == "access_denied"
    assert runtime["recovery_action"] is None


def test_diagnostic_idempotency_and_cancellation(client):
    login(client)
    args = {"json": {"kind": "verify_pipeline"}, "headers": {"Idempotency-Key": "test-diagnostic"}}
    a = client.post("/api/jobs", **args)
    b = client.post("/api/jobs", **args)
    assert a.status_code == 201 and a.json()["id"] == b.json()["id"]
    assert client.post("/api/jobs", json={"kind": "collect"}).status_code == 422
    assert client.post("/api/jobs/" + a.json()["id"] + "/cancel").json()["status"] == "cancelled"


def test_collection_job_cancel_endpoint_atomically_cancels_queued_run(client, database):
    login(client)
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    draft = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "排队取消",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "budget": {"daily_requests": 1000, "content_purchase_cost": 0},
            }
        )
    )
    active = monitors.change_state(
        draft.id,
        MonitorStateChange(expected_version=draft.current_version),
        "active",
    )
    created = CollectionService(database, sources, evidence_configured=True).create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="queued-run-http-cancel",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )

    response = client.post(f"/api/jobs/{created.job_id}/cancel")
    assert response.status_code == 200
    assert response.json()["status"] == "cancelled"
    with database() as session:
        run = session.get(CollectionRun, created.id)
        assert run is not None
        assert run.state == "cancelled" and run.outcome == "partial"
        assert run.stop_reason == "user_cancelled" and run.completed_at is not None


def test_event_api_creates_a_revisioned_empty_dossier(client):
    login(client)
    created = client.post("/api/events", json={"title": "品牌发布会", "summary": "人工整理"})
    assert created.status_code == 201
    event = created.json()
    assert event["current_revision"] == 1 and event["members"] == []
    assert client.get("/api/events").json()["items"][0]["id"] == event["id"]
    revisions = client.get(f"/api/events/{event['id']}/revisions").json()
    assert [revision["change_type"] for revision in revisions] == ["create"]
    trend = client.get(
        f"/api/events/{event['id']}/trends",
        params={
            "since": "2026-09-09T00:00:00Z",
            "until": "2026-09-16T00:00:00Z",
            "bucket_hours": 24,
        },
    )
    assert trend.status_code == 200, trend.json()
    assert trend.json()["metric_version"] == "event-trend-v1"
    assert trend.json()["sources"] == []
    alert_rule = client.post(
        f"/api/events/{event['id']}/trend-alert-rules",
        json={
            "source": "bilibili",
            "metric": "new_posts",
            "bucket_hours": 24,
            "threshold_count": 3,
        },
    )
    assert alert_rule.status_code == 201, alert_rule.json()
    rule = alert_rule.json()
    assert rule["version"] == 1 and rule["enabled"] is True
    assert client.get(f"/api/events/{event['id']}/trend-alert-rules").json() == [rule]
    duplicate_rule = client.post(
        f"/api/events/{event['id']}/trend-alert-rules",
        json={
            "source": "bilibili",
            "metric": "new_posts",
            "bucket_hours": 24,
            "threshold_count": 5,
        },
    )
    assert duplicate_rule.status_code == 409
    assert duplicate_rule.json()["code"] == "trend_alert_rule_exists"
    checked = client.post(f"/api/events/{event['id']}/trend-alerts/evaluate")
    assert checked.status_code == 200
    assert checked.json() == {"evaluated_rules": 1, "created_notifications": 0}
    disabled = client.patch(
        f"/api/events/{event['id']}/trend-alert-rules/{rule['id']}",
        json={"expected_version": 1, "threshold_count": 3, "enabled": False},
    )
    assert disabled.status_code == 200
    assert disabled.json()["version"] == 2 and disabled.json()["enabled"] is False
    stale = client.patch(
        f"/api/events/{event['id']}/trend-alert-rules/{rule['id']}",
        json={"expected_version": 1, "threshold_count": 4, "enabled": True},
    )
    assert stale.status_code == 409
    assert stale.json()["code"] == "trend_alert_rule_version_conflict"
    missing = client.post(
        f"/api/events/{event['id']}/members",
        json={"content_id": "00000000-0000-0000-0000-000000000001"},
    )
    assert missing.status_code == 404 and missing.json()["code"] == "content_not_found"
    source = client.post("/api/events", json={"title": "待合并事件"}).json()
    merged = client.post(
        f"/api/events/{event['id']}/merge",
        json={
            "source_event_id": source["id"],
            "expected_target_revision": event["current_revision"],
            "expected_source_revision": source["current_revision"],
        },
    )
    assert merged.status_code == 200 and merged.json()["current_revision"] == 2
    assert client.get(f"/api/events/{source['id']}").json()["status"] == "archived"
    page = client.get("/api/notifications?unread_only=true").json()
    assert page["unread_count"] == 2
    assert {item["kind"] for item in page["items"]} == {
        "event_merged_in",
        "event_merged_out",
    }
    read = client.post(f"/api/notifications/{page['items'][0]['id']}/read")
    assert read.status_code == 200 and read.json()["read_at"] is not None
    assert client.get("/api/notifications").json()["unread_count"] == 1


def test_login_throttle_persists_failures(client):
    for _ in range(5):
        assert (
            client.post(
                "/api/session", json={"username": "learner", "password": "Wrong-password-123!"}
            ).status_code
            == 401
        )
    response = client.post(
        "/api/session", json={"username": "learner", "password": "Test-password-123!"}
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
                "/api/monitors",
                json={
                    "title": title,
                    "query_spec": {"include_any": ["AI"]},
                    "source_ids": ["bilibili"],
                },
            ).status_code
            == 201
        )
    first = client.get("/api/monitors?limit=2").json()
    second = client.get("/api/monitors", params={"limit": 2, "cursor": first["next_cursor"]}).json()
    assert len({m["id"] for m in first["items"] + second["items"]}) == 3
    assert second["next_cursor"] is None
    with database.begin() as session:
        session.execute(update(LoginSession).values(expires_at=utcnow() - timedelta(seconds=1)))
    assert client.get("/api/session").status_code == 401


def test_unexpected_error_is_sanitized(client, monkeypatch):
    login(client)

    def broken(*_args, **_kwargs):
        raise RuntimeError("private-credential-must-not-escape")

    monkeypatch.setattr(MonitorService, "monitors", broken)
    result = client.get("/api/monitors")
    assert result.status_code == 500
    assert result.json()["code"] == "internal_error"
    assert "private-credential" not in result.text


def test_query_preview_auth_window_and_no_fake_connection(client):
    path = "/api/sources/query-preview"
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
        "/api/monitors",
        json={
            "title": "受控启停",
            "query_spec": {"include_any": ["AI"]},
            "source_ids": ["bilibili"],
            "budget": {"daily_requests": 191, "content_purchase_cost": 0},
        },
    ).json()
    path = f"/api/monitors/{created['id']}"
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
            "budget": {"daily_requests": 192, "content_purchase_cost": 0},
            "expected_version": created["current_version"],
        },
    ).json()
    state = {"expected_version": updated["current_version"]}
    activated = client.post(path + "/activate", json=state)
    assert activated.status_code == 200 and activated.json()["state"] == "active"
    missing_store = client.post(
        path + "/runs",
        json={
            "expected_version": updated["current_version"],
            "idempotency_key": "missing-store",
        },
    )
    assert missing_store.status_code == 409
    assert missing_store.json()["code"] == "evidence_store_not_configured"
    client.app.dependency_overrides[collection_service] = lambda: CollectionService(
        database, evidence_configured=True
    )
    accepted = client.post(
        path + "/runs",
        json={
            "expected_version": updated["current_version"],
            "idempotency_key": "accepted-run",
        },
    )
    assert accepted.status_code == 201
    assert accepted.json()["replayed"] is False
    assert len(accepted.json()["items"]) == 1
    accepted_run = accepted.json()["items"][0]
    run = client.get(f"/api/collection-runs/{accepted_run['id']}")
    assert run.status_code == 200 and run.json()["job_id"] == accepted_run["job_id"]
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
            "budget": {"daily_requests": 384, "content_purchase_cost": 0},
            "expected_version": updated["current_version"],
        },
    ).json()
    assert revised["state"] == "paused" and revised["current_version"] == 3
    reactivated = client.post(
        path + "/activate", json={"expected_version": revised["current_version"]}
    )
    assert reactivated.status_code == 200 and reactivated.json()["state"] == "active"
