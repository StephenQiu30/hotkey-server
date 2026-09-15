from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from api.exception_handlers import register_exception_handlers
from api.middleware import Boundary
from api.router import router
from core.config import Settings
from db.session import Database
from sources.services import SourceService


def create_app(settings: Settings | None = None) -> FastAPI:
    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        app.state.settings = settings or Settings()
        app.state.sources = SourceService.from_settings(app.state.settings)
        database = Database(app.state.settings)
        app.state.database = database
        try:
            yield
        finally:
            database.close()

    app = FastAPI(
        title="HotKey API",
        version="0.2.0",
        docs_url="/docs",
        redoc_url="/redoc",
        openapi_url="/openapi.json",
        lifespan=lifespan,
        description="Single-owner learning workspace. Unsafe requests require an exact allowed "
        "Origin; authenticated writes also require a session-bound X-CSRF-Token. "
        "Monitor configuration history and zero-network query previews are available; "
        "collection remains disabled until a source passes rights and pipeline admission.",
    )
    app.add_middleware(Boundary)

    register_exception_handlers(app)
    app.include_router(router)
    return app
