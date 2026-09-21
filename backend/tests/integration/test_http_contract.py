from uuid import UUID

import pytest
from fastapi import FastAPI, HTTPException, Query
from httpx import ASGITransport, AsyncClient

from core.errors import DependencyUnavailableError
from core.schemas import HealthView


def assert_uuid(value: str) -> None:
    assert str(UUID(value)) == value


@pytest.mark.anyio
async def test_health_contract(app: FastAPI) -> None:
    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/api/health")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
    assert response.headers["x-request-id"]


@pytest.mark.anyio
async def test_http_exception_is_normalized_and_preserves_headers(app: FastAPI) -> None:
    def raise_http_error() -> HealthView:
        raise HTTPException(
            status_code=401,
            detail="需要登录",
            headers={"WWW-Authenticate": "Bearer"},
        )

    app.add_api_route(
        "/__test/http-error",
        raise_http_error,
        methods=["GET"],
        include_in_schema=False,
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/__test/http-error")

    assert response.status_code == 401
    assert response.json() == {
        "code": "unauthorized",
        "message": "需要登录",
        "request_id": response.headers["x-request-id"],
    }
    assert response.headers["www-authenticate"] == "Bearer"


@pytest.mark.anyio
async def test_http_exception_rejects_untrusted_headers_and_server_detail(app: FastAPI) -> None:
    def raise_http_error() -> HealthView:
        raise HTTPException(
            status_code=500,
            detail="token=should-not-leak",
            headers={
                "Set-Cookie": "session=should-not-leak",
                "X-Injected": "should-not-leak",
            },
        )

    app.add_api_route(
        "/__test/http-server-error",
        raise_http_error,
        methods=["GET"],
        include_in_schema=False,
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app, raise_app_exceptions=False),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/__test/http-server-error")

    assert response.status_code == 500
    assert response.json() == {
        "code": "internal_error",
        "message": "服务暂时不可用",
        "request_id": response.headers["x-request-id"],
    }
    assert "set-cookie" not in response.headers
    assert "x-injected" not in response.headers
    assert "should-not-leak" not in response.text


@pytest.mark.anyio
async def test_not_found_is_normalized(app: FastAPI) -> None:
    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/api/does-not-exist")

    assert response.status_code == 404
    assert response.json() == {
        "code": "not_found",
        "message": "请求资源不存在",
        "request_id": response.headers["x-request-id"],
    }


@pytest.mark.anyio
async def test_application_error_is_normalized(app: FastAPI) -> None:
    def raise_application_error() -> HealthView:
        raise DependencyUnavailableError()

    app.add_api_route(
        "/__test/application-error",
        raise_application_error,
        methods=["GET"],
        include_in_schema=False,
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/__test/application-error")

    assert response.status_code == 503
    assert response.json() == {
        "code": "database_unavailable",
        "message": "数据库暂不可用",
        "request_id": response.headers["x-request-id"],
    }


@pytest.mark.anyio
async def test_request_validation_error_is_normalized_with_safe_field_details(
    app: FastAPI,
) -> None:
    def validate_limit(limit: int = Query(ge=1)) -> HealthView:
        return HealthView(status="ok")

    app.add_api_route(
        "/__test/validation",
        validate_limit,
        methods=["GET"],
        include_in_schema=False,
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/__test/validation", params={"limit": "secret"})

    assert response.status_code == 422
    body = response.json()
    assert body["code"] == "validation_error"
    assert body["request_id"] == response.headers["x-request-id"]
    assert body["details"] == [
        {
            "location": ["query", "limit"],
            "message": "请输入有效整数",
            "type": "int_parsing",
        }
    ]
    assert "secret" not in response.text


@pytest.mark.anyio
async def test_unexpected_exception_is_safe_and_has_request_id(
    app: FastAPI,
    capsys: pytest.CaptureFixture[str],
) -> None:
    def raise_unexpected_error() -> HealthView:
        raise RuntimeError("database password=should-not-leak")

    app.add_api_route(
        "/__test/unexpected-error",
        raise_unexpected_error,
        methods=["GET"],
        include_in_schema=False,
    )

    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app, raise_app_exceptions=False),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/__test/unexpected-error")

    assert response.status_code == 500
    assert response.json() == {
        "code": "internal_error",
        "message": "服务暂时不可用",
        "request_id": response.headers["x-request-id"],
    }
    assert_uuid(response.headers["x-request-id"])
    assert "database password" not in response.text
    assert "database password" not in capsys.readouterr().out


@pytest.mark.anyio
async def test_openapi_and_documentation_share_runtime_contract(app: FastAPI) -> None:
    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app),
            base_url="http://test",
        ) as client,
    ):
        schema = await client.get("/openapi.json")
        swagger = await client.get("/docs")
        scalar = await client.get("/scalar")

    assert schema.status_code == 200
    assert schema.json()["paths"]["/api/health"]["get"]["operationId"] == "getHealth"
    assert "/scalar" not in schema.json()["paths"]
    assert swagger.status_code == 200
    assert scalar.status_code == 200
