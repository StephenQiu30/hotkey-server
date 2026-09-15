import logging

from fastapi.testclient import TestClient

from core.config import Settings
from main import create_app


def test_request_completion_log_uses_route_template_without_query(caplog):
    settings = Settings(
        database_url="postgresql+psycopg://u:p@127.0.0.1:1/db",
        broker_url="amqp://u:p@127.0.0.1:1/test",
    )
    caplog.set_level(logging.INFO, logger="api.middleware")

    with TestClient(create_app(settings)) as client:
        response = client.get("/health/live?token=must-not-be-logged")

    assert response.status_code == 200
    completed = [
        record.message for record in caplog.records if "http_request_completed" in record.message
    ]
    assert len(completed) == 1
    assert "method=GET" in completed[0]
    assert "route=/health/live" in completed[0]
    assert "status_code=200" in completed[0]
    assert "must-not-be-logged" not in completed[0]
