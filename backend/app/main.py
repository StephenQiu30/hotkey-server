from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from api.docs import register_documentation
from api.exception_handlers import register_exception_handlers
from api.middleware import register_middleware
from api.router import api_router
from core.config import Settings, get_settings
from core.logging import configure_logging
from db.session import create_db_engine, create_session_factory


def create_app(settings: Settings | None = None) -> FastAPI:
    resolved_settings = settings or get_settings()
    configure_logging(resolved_settings.log_level)

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        engine = create_db_engine(resolved_settings)
        app.state.session_factory = create_session_factory(engine)
        try:
            yield
        finally:
            engine.dispose()

    app = FastAPI(
        title=resolved_settings.app_name,
        version=resolved_settings.app_version,
        openapi_url="/openapi.json",
        docs_url="/docs",
        redoc_url=None,
        lifespan=lifespan,
    )
    app.state.settings = resolved_settings
    register_middleware(app)
    register_exception_handlers(app)
    register_documentation(app)
    app.include_router(api_router)
    return app
