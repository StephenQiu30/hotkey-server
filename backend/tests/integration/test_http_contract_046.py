from __future__ import annotations

import asyncio
import json
from uuid import UUID

import pytest
from fastapi import FastAPI, Query, Response
from httpx import ASGITransport, AsyncClient

from core.config import Settings
from core.schemas import ErrorView, HealthView, JobAcceptedView, PageView
from main import create_app


def _assert_uuid(value: str) -> None:
    assert str(UUID(value)) == value


@pytest.mark.anyio
async def test_concurrent_requests_use_distinct_request_ids(app: FastAPI) -> None:
    async with (
        app.router.lifespan_context(app),
        AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client,
    ):
        responses = await asyncio.gather(*(client.get("/api/health") for _ in range(20)))

    request_ids = [response.headers["x-request-id"] for response in responses]
    assert len(set(request_ids)) == len(request_ids)
    for request_id in request_ids:
        _assert_uuid(request_id)


@pytest.mark.anyio
async def test_request_id_matches_error_body_and_access_log(
    capsys: pytest.CaptureFixture[str],
) -> None:
    app = create_app(
        Settings(
            environment="test",
            log_level="INFO",
            database_url="postgresql+psycopg://test:test@127.0.0.1:5432/hotkey_test",
        )
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client,
    ):
        response = await client.get("/missing")

    request_id = response.headers["x-request-id"]
    log_entries = [
        json.loads(line) for line in capsys.readouterr().out.splitlines() if line.startswith("{")
    ]
    access_log = next(entry for entry in log_entries if entry["event"] == "request_completed")

    _assert_uuid(request_id)
    assert response.json()["request_id"] == request_id
    assert access_log["request_id"] == request_id
    assert access_log["route"] == "unmatched"
    assert "raw_path" not in access_log


@pytest.mark.anyio
async def test_access_log_uses_the_full_route_template(
    capsys: pytest.CaptureFixture[str],
) -> None:
    app = create_app(
        Settings(
            environment="test",
            log_level="INFO",
            database_url="postgresql+psycopg://test:test@127.0.0.1:5432/hotkey_test",
        )
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client,
    ):
        response = await client.get("/api/health?token=should-not-be-logged")

    log_entries = [
        json.loads(line) for line in capsys.readouterr().out.splitlines() if line.startswith("{")
    ]
    access_log = next(entry for entry in log_entries if entry["event"] == "request_completed")

    assert response.status_code == 200
    assert access_log["route"] == "/health"
    assert "should-not-be-logged" not in json.dumps(access_log)


@pytest.mark.anyio
async def test_response_validation_failure_is_safe(
    app: FastAPI,
    capsys: pytest.CaptureFixture[str],
) -> None:
    def invalid_response() -> dict[str, str]:
        return {"status": "token=should-not-leak"}

    app.add_api_route(
        "/__test/invalid-response",
        invalid_response,
        methods=["GET"],
        response_model=HealthView,
        include_in_schema=False,
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app, raise_app_exceptions=False),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/__test/invalid-response")

    assert response.status_code == 500
    assert response.headers["cache-control"] == "no-store"
    assert response.json()["code"] == "internal_error"
    assert response.json()["request_id"] == response.headers["x-request-id"]
    assert "should-not-leak" not in response.text
    assert "should-not-leak" not in capsys.readouterr().out


@pytest.mark.anyio
async def test_response_families_preserve_protocol_semantics(app: FastAPI) -> None:
    job_id = UUID("5a349627-c092-4a9d-9d0f-8fe465cdebee")

    def get_page() -> PageView[HealthView]:
        return PageView(items=[HealthView(status="ok")], next_cursor=None)

    def accept_job() -> JobAcceptedView:
        return JobAcceptedView(job_id=job_id, status="queued")

    def no_content() -> Response:
        return Response(status_code=204)

    def not_modified() -> Response:
        return Response(status_code=304)

    def download_file() -> Response:
        return Response(content=b"\x00hotkey", media_type="application/octet-stream")

    app.add_api_route(
        "/__test/page",
        get_page,
        methods=["GET"],
        response_model=PageView[HealthView],
        status_code=200,
        include_in_schema=False,
    )
    app.add_api_route(
        "/__test/jobs",
        accept_job,
        methods=["POST"],
        response_model=JobAcceptedView,
        status_code=202,
        include_in_schema=False,
    )
    app.add_api_route(
        "/__test/no-content",
        no_content,
        methods=["DELETE"],
        response_model=None,
        status_code=204,
        include_in_schema=False,
    )
    app.add_api_route(
        "/__test/not-modified",
        not_modified,
        methods=["GET"],
        response_model=None,
        status_code=304,
        include_in_schema=False,
    )
    app.add_api_route(
        "/__test/file",
        download_file,
        methods=["GET"],
        response_model=None,
        status_code=200,
        include_in_schema=False,
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client,
    ):
        page, accepted, empty, cached, file_response = await asyncio.gather(
            client.get("/__test/page"),
            client.post("/__test/jobs"),
            client.delete("/__test/no-content"),
            client.get("/__test/not-modified"),
            client.get("/__test/file"),
        )

    assert page.json() == {"items": [{"status": "ok"}], "next_cursor": None}
    assert accepted.status_code == 202
    assert accepted.json() == {"job_id": str(job_id), "status": "queued"}
    assert empty.status_code == 204 and empty.content == b""
    assert cached.status_code == 304 and cached.content == b""
    assert file_response.content == b"\x00hotkey"
    assert file_response.headers["content-type"] == "application/octet-stream"


@pytest.mark.anyio
async def test_openapi_validation_errors_use_error_view(app: FastAPI) -> None:
    def validate_limit(limit: int = Query(ge=1)) -> HealthView:
        return HealthView(status="ok")

    app.add_api_route(
        "/__test/openapi-validation",
        validate_limit,
        methods=["GET"],
        operation_id="testOpenApiValidation",
        response_model=HealthView,
        responses={422: {"model": ErrorView, "description": "请求参数校验失败"}},
        status_code=200,
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client,
    ):
        schema_response = await client.get("/openapi.json")
        invalid_response = await client.get(
            "/__test/openapi-validation",
            params={"limit": "secret"},
        )

    schema = schema_response.json()
    validation_schema = schema["paths"]["/__test/openapi-validation"]["get"]["responses"]["422"]
    response_schema = validation_schema["content"]["application/json"]["schema"]

    assert response_schema == {"$ref": "#/components/schemas/ErrorView"}
    assert "HTTPValidationError" not in schema["components"]["schemas"]
    assert invalid_response.json()["code"] == "validation_error"
    assert invalid_response.headers["cache-control"] == "no-store"
    assert invalid_response.json()["request_id"] == invalid_response.headers["x-request-id"]
