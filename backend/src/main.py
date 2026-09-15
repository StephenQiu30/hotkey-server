from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from fastapi import FastAPI

from api.exception_handlers import register_exception_handlers
from api.middleware import Boundary
from api.router import router
from core.config import Settings
from core.schemas import ErrorView
from db.session import Database


def create_app(settings: Settings | None = None) -> FastAPI:
    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        app.state.settings = settings or Settings()
        database = Database(app.state.settings)
        app.state.database = database
        try:
            yield
        finally:
            database.close()

    errors: dict[int | str, dict[str, Any]] = {
        status: {"model": ErrorView}
        for status in (400, 401, 403, 404, 409, 413, 422, 429, 500, 503)
    }
    app = FastAPI(
        title="HotKey API",
        version="0.2.0",
        docs_url="/docs",
        redoc_url="/redoc",
        openapi_url="/openapi.json",
        lifespan=lifespan,
        responses=errors,
        description="Single-owner learning workspace. Unsafe requests require an exact allowed "
        "Origin; authenticated writes also require a session-bound X-CSRF-Token. "
        "Monitor configuration history and zero-network query previews are available; "
        "collection remains disabled until a source passes rights and pipeline admission.",
    )
    app.add_middleware(Boundary)

    register_exception_handlers(app)
    app.include_router(router)
    return app
