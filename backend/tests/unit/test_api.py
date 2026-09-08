from fastapi.testclient import TestClient

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
