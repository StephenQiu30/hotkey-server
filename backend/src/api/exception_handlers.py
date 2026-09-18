from __future__ import annotations

import structlog
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from structlog.contextvars import get_contextvars

from core.errors import ApplicationError, DependencyUnavailableError
from core.schemas import ErrorView


def _request_id() -> str:
    return str(get_contextvars().get("request_id", "unknown"))


def register_exception_handlers(app: FastAPI) -> None:
    @app.exception_handler(ApplicationError)
    async def application_error_handler(_: Request, error: ApplicationError) -> JSONResponse:
        status_code = 503 if isinstance(error, DependencyUnavailableError) else 400
        view = ErrorView(code=error.code, message=error.message, request_id=_request_id())
        return JSONResponse(status_code=status_code, content=view.model_dump())

    @app.exception_handler(Exception)
    async def unexpected_error_handler(_: Request, error: Exception) -> JSONResponse:
        structlog.get_logger("api").exception(
            "unhandled_exception",
            exception_type=type(error).__name__,
        )
        view = ErrorView(
            code="internal_error",
            message="服务暂时不可用",
            request_id=_request_id(),
        )
        return JSONResponse(status_code=500, content=view.model_dump())
