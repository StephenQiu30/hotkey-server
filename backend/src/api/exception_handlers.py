from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from sqlalchemy.exc import IntegrityError, SQLAlchemyError
from starlette.exceptions import HTTPException

from core.errors import AppError


def register_exception_handlers(app: FastAPI) -> None:
    @app.exception_handler(AppError)
    async def domain_error(request: Request, exc: AppError) -> JSONResponse:
        return JSONResponse(
            {"code": exc.code, "request_id": request.state.request_id},
            exc.status,
            headers={"Retry-After": "900"} if exc.status == 429 else None,
        )

    @app.exception_handler(HTTPException)
    async def http_error(request: Request, exc: HTTPException) -> JSONResponse:
        return JSONResponse(
            {
                "code": "not_found" if exc.status_code == 404 else "http_error",
                "request_id": request.state.request_id,
            },
            exc.status_code,
            headers=exc.headers,
        )

    @app.exception_handler(RequestValidationError)
    async def invalid(request: Request, exc: RequestValidationError) -> JSONResponse:
        return JSONResponse(
            {"code": "validation_failed", "request_id": request.state.request_id}, 422
        )

    @app.exception_handler(SQLAlchemyError)
    async def database_error(request: Request, exc: SQLAlchemyError) -> JSONResponse:
        conflict = isinstance(exc, IntegrityError)
        return JSONResponse(
            {
                "code": "write_conflict" if conflict else "database_unavailable",
                "request_id": request.state.request_id,
            },
            409 if conflict else 503,
        )
