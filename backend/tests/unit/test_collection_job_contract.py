from __future__ import annotations

from types import SimpleNamespace
from uuid import uuid4

from fastapi.testclient import TestClient

from api.dependencies import get_job_service, get_webpage_collection_service, require_identity_csrf
from core.config import Settings
from main import create_app


def test_collection_job_contract_only_exposes_registered_worker_handler() -> None:
    app = create_app(
        Settings(
            environment="test",
            log_level="WARNING",
            database_url="postgresql+psycopg://unused:unused@127.0.0.1/unopened",
            bootstrap_token="test-only-bootstrap-token-used-here",
        )
    )
    accepted: list[bool] = []
    app.dependency_overrides[require_identity_csrf] = lambda: SimpleNamespace(
        view=SimpleNamespace(user=SimpleNamespace(id=uuid4()))
    )
    app.dependency_overrides[get_webpage_collection_service] = lambda: object()
    app.dependency_overrides[get_job_service] = lambda: SimpleNamespace(
        accept=lambda **_: accepted.append(True) or SimpleNamespace(id=uuid4())
    )

    schema = app.openapi()
    request_schema = schema["paths"]["/api/jobs"]["post"]["requestBody"]["content"][
        "application/json"
    ]["schema"]

    assert request_schema == {"$ref": "#/components/schemas/WebPageCollectionJobInput"}
    assert "CollectionJobInput" not in schema["components"]["schemas"]

    response = TestClient(app).post(
        "/api/jobs",
        json={
            "operation_id": str(uuid4()),
            "kind": "monitor.collect",
            "observation": {
                "configuration_ref": "monitor-config-1",
                "configuration_version": 1,
                "source_key": "x",
                "source_capability": "search",
            },
            "scheduled_for_at": None,
            "scope": {"query": "brand"},
        },
    )

    assert response.status_code == 422
    assert response.json()["code"] == "validation_error"
    assert accepted == []
