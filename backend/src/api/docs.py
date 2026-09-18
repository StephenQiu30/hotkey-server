from fastapi import FastAPI
from scalar_fastapi import get_scalar_api_reference
from starlette.responses import HTMLResponse


def register_documentation(app: FastAPI) -> None:
    @app.get("/scalar", include_in_schema=False)
    async def scalar_documentation() -> HTMLResponse:
        return get_scalar_api_reference(
            openapi_url=app.openapi_url or "/openapi.json",
            title=f"{app.title} API",
        )
