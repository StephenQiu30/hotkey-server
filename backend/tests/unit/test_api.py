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
        assert client.get("/api/monitors").status_code == 401
        legacy = client.get("/api/v" + "1/monitors", follow_redirects=False)
        assert legacy.status_code == 404
        assert "location" not in legacy.headers


def test_contract_declares_session_security():
    document = create_app().openapi()
    assert document["components"]["securitySchemes"]["OwnerSession"] == {
        "type": "apiKey",
        "in": "cookie",
        "name": "hk_session",
    }
    assert document["paths"]["/api/monitors"]["get"]["security"] == [{"OwnerSession": []}]
    assert "security" not in document["paths"]["/api/session"]["post"]


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
        ("/api/session", "post"): "login",
        ("/api/session", "get"): "getSession",
        ("/api/session", "delete"): "logout",
        ("/api/sources", "get"): "listSources",
        ("/api/sources/query-preview", "post"): "previewSourceQueries",
        ("/api/monitors", "get"): "listMonitors",
        ("/api/monitors", "post"): "createMonitor",
        ("/api/monitors/{identity}", "patch"): "updateMonitor",
        ("/api/monitors/{identity}/activate", "post"): "activateMonitor",
        ("/api/monitors/{identity}/pause", "post"): "pauseMonitor",
        ("/api/monitors/{identity}/runs", "post"): "createCollectionRun",
        ("/api/collection-runs", "get"): "listCollectionRuns",
        ("/api/collection-runs/{identity}", "get"): "getCollectionRun",
        ("/api/contents", "get"): "listInboxContents",
        ("/api/events", "get"): "listEvents",
        ("/api/events", "post"): "createEvent",
        ("/api/events/{identity}", "get"): "getEvent",
        ("/api/events/{identity}/members", "post"): "addEventMember",
        ("/api/events/{identity}/members/{content_id}", "delete"): "removeEventMember",
        ("/api/knowledge/query", "post"): "queryKnowledge",
        ("/api/contents/{identity}/withdraw", "post"): "withdrawContent",
        ("/api/events/{identity}/revisions", "get"): "listEventRevisions",
        ("/api/events/{identity}/merge", "post"): "mergeEvent",
        ("/api/events/{identity}/split", "post"): "splitEvent",
        ("/api/events/{identity}/trends", "get"): "getEventTrends",
        ("/api/events/{identity}/analysis-runs", "post"): "createEventAnalysisRun",
        ("/api/events/{identity}/analysis-runs", "get"): "listEventAnalysisRuns",
        ("/api/analysis-runs/{identity}", "get"): "getAnalysisRun",
        ("/api/analysis-runs/{identity}/recompute", "post"): "recomputeAnalysisRun",
        (
            "/api/analysis-runs/{identity}/knowledge-entry",
            "post",
        ): "publishAnalysisKnowledge",
        (
            "/api/analysis-runs/{identity}/samples/{sample_id}/label",
            "put",
        ): "labelAnalysisSample",
        ("/api/knowledge", "get"): "searchKnowledge",
        ("/api/knowledge/{identity}", "get"): "getKnowledgeEntry",
        (
            "/api/knowledge/{identity}/semantic-index",
            "post",
        ): "indexKnowledgeEntry",
        ("/api/notifications", "get"): "listNotifications",
        ("/api/notifications/{identity}/read", "post"): "markNotificationRead",
        ("/api/jobs", "get"): "listJobs",
        ("/api/jobs", "post"): "createDiagnosticJob",
        ("/api/jobs/{identity}", "get"): "getJob",
        ("/api/jobs/{identity}/cancel", "post"): "cancelJob",
    }
    document = create_app().openapi()
    actual = {
        (path, method): operation["operationId"]
        for path, methods in document["paths"].items()
        for method, operation in methods.items()
    }
    assert actual == expected
    assert (
        document["paths"]["/api/events"]["post"]["responses"]["413"]["description"]
        == "Request content too large"
    )
    assert document["info"]["title"] == "HotKey API"
    run_input = document["components"]["schemas"]["CollectionRunRequest"]
    run_view = document["components"]["schemas"]["CollectionRunView"]
    assert "request_value" in run_input["required"]
    assert "ingestion_mode" in run_input["required"]
    assert "query_variant" not in run_input["properties"]
    assert run_view["properties"]["operation"]["enum"] == [
        "search_posts",
        "fetch_post",
        "list_comments",
        "list_replies",
    ]
    assert "parent_run_id" in run_view["required"]

    with TestClient(create_app()) as client:
        swagger = client.get("/docs")
        assert swagger.status_code == 200
        assert "url: '/openapi.json'" in swagger.text
        assert client.get("/openapi.json").json()["info"]["title"] == "HotKey API"
