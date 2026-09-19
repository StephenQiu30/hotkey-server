from __future__ import annotations

from collections.abc import Mapping
from typing import Any

import structlog
from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException
from structlog.contextvars import get_contextvars

from core.errors import ApplicationError
from core.schemas import ErrorView, ValidationErrorItem


def _request_id() -> str:
    return str(get_contextvars().get("request_id", "unknown"))


def _error_response(
    *,
    status_code: int,
    code: str,
    message: str,
    headers: Mapping[str, str] | None = None,
    details: list[ValidationErrorItem] | None = None,
) -> JSONResponse:
    view = ErrorView(
        code=code,
        message=message,
        request_id=_request_id(),
        details=details,
    )
    response_headers = dict(headers or {})
    response_headers["x-request-id"] = view.request_id
    return JSONResponse(
        status_code=status_code,
        content=view.model_dump(exclude_none=True),
        headers=response_headers,
    )


def _http_error_code(status_code: int) -> str:
    return {
        400: "bad_request",
        401: "unauthorized",
        403: "forbidden",
        404: "not_found",
        405: "method_not_allowed",
        409: "conflict",
        429: "rate_limited",
    }.get(status_code, "http_error")


def _http_error_message(status_code: int, detail: Any) -> str:
    default_detail = {
        400: "Bad Request",
        401: "Unauthorized",
        403: "Forbidden",
        404: "Not Found",
        405: "Method Not Allowed",
        409: "Conflict",
        429: "Too Many Requests",
    }.get(status_code)
    if isinstance(detail, str) and detail and detail != default_detail:
        return detail
    return {
        400: "请求无效",
        401: "需要登录",
        403: "没有权限",
        404: "请求资源不存在",
        405: "请求方法不支持",
        409: "请求与当前资源状态冲突",
        429: "请求过于频繁",
    }.get(status_code, "请求无法处理")


def _validation_details(error: RequestValidationError) -> list[ValidationErrorItem]:
    details: list[ValidationErrorItem] = []
    for item in error.errors():
        location = [part if isinstance(part, (str, int)) else str(part) for part in item["loc"]]
        details.append(
            ValidationErrorItem(
                location=location,
                message=str(item["msg"]),
                type=str(item["type"]),
            )
        )
    return details


def register_exception_handlers(app: FastAPI) -> None:
    @app.exception_handler(ApplicationError)
    async def application_error_handler(_: Request, error: ApplicationError) -> JSONResponse:
        return _error_response(
            status_code=error.status_code,
            code=error.code,
            message=error.message,
        )

    @app.exception_handler(StarletteHTTPException)
    async def http_exception_handler(
        _: Request,
        error: StarletteHTTPException,
    ) -> JSONResponse:
        return _error_response(
            status_code=error.status_code,
            code=_http_error_code(error.status_code),
            message=_http_error_message(error.status_code, error.detail),
            headers=error.headers,
        )

    @app.exception_handler(RequestValidationError)
    async def request_validation_error_handler(
        _: Request,
        error: RequestValidationError,
    ) -> JSONResponse:
        return _error_response(
            status_code=422,
            code="validation_error",
            message="请求参数校验失败",
            details=_validation_details(error),
        )

    @app.exception_handler(Exception)
    async def unexpected_error_handler(_: Request, error: Exception) -> JSONResponse:
        structlog.get_logger("api").exception(
            "unhandled_exception",
            exception_type=type(error).__name__,
        )
        return _error_response(
            status_code=500,
            code="internal_error",
            message="服务暂时不可用",
        )
