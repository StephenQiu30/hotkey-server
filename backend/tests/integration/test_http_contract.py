import pytest
from fastapi import FastAPI
from httpx import ASGITransport, AsyncClient


@pytest.mark.anyio
async def test_health_contract(app: FastAPI) -> None:
    async with (
        app.router.lifespan_context(app),
        AsyncClient(
            transport=ASGITransport(app=app),
            base_url="http://test",
        ) as client,
    ):
        response = await client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
    assert response.headers["x-request-id"]


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
    assert schema.json()["paths"]["/health"]["get"]["operationId"] == "getHealth"
    assert "/scalar" not in schema.json()["paths"]
    assert swagger.status_code == 200
    assert scalar.status_code == 200
