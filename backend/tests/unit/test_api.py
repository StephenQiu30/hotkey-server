import pytest
from fastapi.testclient import TestClient

from core.config import Settings
from main import create_app


def test_live_does_not_claim_pipeline_or_collection_ready(monkeypatch):
    monkeypatch.setenv("HOTKEY_DATABASE_URL", "postgresql+psycopg://u:p@127.0.0.1:1/db")
    monkeypatch.setenv("HOTKEY_BROKER_URL", "amqp://u:p@127.0.0.1:1/test")
    with TestClient(create_app()) as client:
        assert client.get("/health/live").json() == {"status": "alive"}
        result = client.get("/health/ready")
        assert result.status_code == 503
        assert result.json() == {"status": "not_ready", "code": "database_unavailable"}
        assert client.get("/api/v1/monitors").status_code == 401


def test_contract_declares_session_security():
    document = create_app().openapi()
    assert document["components"]["securitySchemes"]["OwnerSession"] == {
        "type": "apiKey",
        "in": "cookie",
        "name": "hk_session",
    }
    assert document["paths"]["/api/v1/monitors"]["get"]["security"] == [{"OwnerSession": []}]
    assert "security" not in document["paths"]["/api/v1/session"]["post"]


def test_api_startup_rejects_unknown_source_operation():
    settings = Settings(
        database_url="postgresql+psycopg://u:p@127.0.0.1:1/db",
        broker_url="amqp://u:p@127.0.0.1:1/test",
        source_rights_allowed=["unknown.search_posts"],
    )
    with pytest.raises(ValueError, match="unknown source operation"):
        with TestClient(create_app(settings)):
            pass


def test_swagger_contract_has_stable_client_operation_ids(monkeypatch):
    monkeypatch.setenv("HOTKEY_DATABASE_URL", "postgresql+psycopg://u:p@127.0.0.1:1/db")
    monkeypatch.setenv("HOTKEY_BROKER_URL", "amqp://u:p@127.0.0.1:1/test")
    expected = {
        ("/health/live", "get"): "healthLive",
        ("/health/ready", "get"): "healthReady",
        ("/api/v1/session", "post"): "login",
        ("/api/v1/session", "get"): "getSession",
        ("/api/v1/session", "delete"): "logout",
        ("/api/v1/sources", "get"): "listSources",
        ("/api/v1/sources/query-preview", "post"): "previewSourceQueries",
        ("/api/v1/monitors", "get"): "listMonitors",
        ("/api/v1/monitors", "post"): "createMonitor",
        ("/api/v1/monitors/{identity}", "patch"): "updateMonitor",
        ("/api/v1/monitors/{identity}/activate", "post"): "activateMonitor",
        ("/api/v1/monitors/{identity}/pause", "post"): "pauseMonitor",
        ("/api/v1/monitors/{identity}/runs", "post"): "createCollectionRun",
        ("/api/v1/collection-runs", "get"): "listCollectionRuns",
        ("/api/v1/collection-runs/{identity}", "get"): "getCollectionRun",
        ("/api/v1/contents", "get"): "listInboxContents",
        ("/api/v1/jobs", "get"): "listJobs",
        ("/api/v1/jobs", "post"): "createDiagnosticJob",
        ("/api/v1/jobs/{identity}", "get"): "getJob",
        ("/api/v1/jobs/{identity}/cancel", "post"): "cancelJob",
    }
    document = create_app().openapi()
    actual = {
        (path, method): operation["operationId"]
        for path, methods in document["paths"].items()
        for method, operation in methods.items()
    }
    assert actual == expected
    assert document["info"]["title"] == "HotKey API"

    with TestClient(create_app()) as client:
        swagger = client.get("/docs")
        assert swagger.status_code == 200
        assert "url: '/openapi.json'" in swagger.text
        assert client.get("/openapi.json").json()["info"]["title"] == "HotKey API"
