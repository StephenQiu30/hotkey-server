from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from traceback import extract_tb
from typing import Any
from uuid import UUID, uuid4

import structlog
from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError, ResponseValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from core.errors import ERROR_CATEGORIES, ApplicationError, ErrorCategory
from core.schemas import ErrorView, ValidationErrorItem


@dataclass(frozen=True, slots=True)
class PublicError:
    status_code: int
    code: str
    message: str


INTERNAL_ERROR = PublicError(500, "internal_error", "服务暂时不可用")
HTTP_ERRORS: Mapping[int, PublicError] = {
    400: PublicError(400, "bad_request", "请求无效"),
    401: PublicError(401, "unauthorized", "需要登录"),
    403: PublicError(403, "forbidden", "没有权限"),
    404: PublicError(404, "not_found", "请求资源不存在"),
    405: PublicError(405, "method_not_allowed", "请求方法不支持"),
    409: PublicError(409, "conflict", "请求与当前资源状态冲突"),
    413: PublicError(413, "payload_too_large", "请求内容过大"),
    415: PublicError(415, "unsupported_media_type", "请求格式不受支持"),
    422: PublicError(422, "validation_error", "请求参数校验失败"),
    429: PublicError(429, "rate_limited", "请求过于频繁"),
    500: INTERNAL_ERROR,
    502: PublicError(502, "upstream_error", "上游服务响应异常"),
    503: PublicError(503, "dependency_unavailable", "必要依赖暂不可用"),
    504: PublicError(504, "upstream_timeout", "上游服务响应超时"),
}
APPLICATION_ERRORS: Mapping[str, PublicError] = {
    "bootstrap_forbidden": PublicError(403, "bootstrap_forbidden", "初始化授权无效"),
    "csrf_invalid": PublicError(403, "csrf_invalid", "请求安全校验失败"),
    "database_unavailable": PublicError(503, "database_unavailable", "数据库暂不可用"),
    "identity_already_initialized": PublicError(
        409,
        "identity_already_initialized",
        "使用者已完成初始化",
    ),
    "identity_not_initialized": PublicError(404, "identity_not_initialized", "使用者尚未初始化"),
    "idempotency_conflict": PublicError(409, "idempotency_conflict", "操作标识已用于其他请求"),
    "invalid_credentials": PublicError(401, "invalid_credentials", "用户名或密码错误"),
    "invalid_password": PublicError(422, "invalid_password", "密码不符合安全要求"),
    "invalid_session": PublicError(401, "invalid_session", "会话无效或已过期"),
    "resource_not_found": PublicError(404, "resource_not_found", "请求资源不存在"),
}

_ALLOWED_HEADERS: Mapping[int, frozenset[str]] = {
    401: frozenset({"www-authenticate"}),
    405: frozenset({"allow"}),
    429: frozenset({"retry-after"}),
    503: frozenset({"retry-after"}),
}
_SAFE_FIELD_NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]{0,63}$")
_VALIDATION_MESSAGES: Mapping[str, str] = {
    "bool_parsing": "请输入有效布尔值",
    "date_parsing": "请输入有效日期",
    "datetime_parsing": "请输入有效日期时间",
    "extra_forbidden": "包含不允许的字段",
    "float_parsing": "请输入有效数字",
    "greater_than": "输入值必须更大",
    "greater_than_equal": "输入值过小",
    "int_parsing": "请输入有效整数",
    "less_than": "输入值必须更小",
    "less_than_equal": "输入值过大",
    "list_type": "请输入有效列表",
    "literal_error": "请选择有效值",
    "missing": "此字段为必填项",
    "string_too_long": "输入内容过长",
    "string_too_short": "输入内容过短",
    "string_type": "请输入有效文本",
    "uuid_parsing": "请输入有效标识",
    "value_error": "输入值不合法",
}


def _request_id(request: Request) -> UUID:
    candidate = getattr(request.state, "request_id", None)
    try:
        return UUID(str(candidate))
    except (TypeError, ValueError, AttributeError):
        request_id = uuid4()
        request.state.request_id = str(request_id)
        return request_id


def _safe_headers(status_code: int, headers: Mapping[str, str] | None) -> dict[str, str]:
    allowed = _ALLOWED_HEADERS.get(status_code, frozenset())
    safe: dict[str, str] = {}
    for name, value in (headers or {}).items():
        normalized = name.lower()
        if normalized not in allowed or "\r" in value or "\n" in value:
            continue
        safe[name] = value
    return safe


def _error_response(
    request: Request,
    error: PublicError,
    *,
    headers: Mapping[str, str] | None = None,
    details: list[ValidationErrorItem] | None = None,
) -> JSONResponse:
    view = ErrorView(
        code=error.code,
        message=error.message,
        request_id=_request_id(request),
        details=details,
    )
    response_headers = _safe_headers(error.status_code, headers)
    response_headers["x-request-id"] = str(view.request_id)
    return JSONResponse(
        status_code=error.status_code,
        content=view.model_dump(mode="json", exclude_none=True),
        headers=response_headers,
    )


def _http_error(status_code: int) -> PublicError:
    if status_code >= 500:
        return INTERNAL_ERROR
    return HTTP_ERRORS.get(
        status_code,
        PublicError(status_code, "http_error", "请求无法处理"),
    )


def _safe_location(location: tuple[Any, ...], error_type: str) -> list[str | int]:
    safe: list[str | int] = []
    for index, part in enumerate(location):
        if isinstance(part, int) or (
            index == 0 and part in {"body", "cookie", "header", "path", "query"}
        ):
            safe.append(part)
        elif isinstance(part, str) and _SAFE_FIELD_NAME.fullmatch(part):
            safe.append("<field>" if error_type == "extra_forbidden" else part)
        else:
            safe.append("<field>")
    return safe


def _validation_details(error: RequestValidationError) -> list[ValidationErrorItem]:
    details: list[ValidationErrorItem] = []
    for item in error.errors():
        error_type = str(item["type"])
        details.append(
            ValidationErrorItem(
                location=_safe_location(tuple(item["loc"]), error_type),
                message=_VALIDATION_MESSAGES.get(error_type, "输入值不合法"),
                type=error_type,
            )
        )
    return details


def _exception_location(error: Exception) -> str:
    frames = extract_tb(error.__traceback__)
    if not frames:
        return "unknown"
    frame = frames[-1]
    return f"{Path(frame.filename).name}:{frame.lineno}:{frame.name}"


def _log_unexpected(request: Request, error: Exception, event: str) -> None:
    structlog.get_logger("api").error(
        event,
        request_id=str(_request_id(request)),
        exception_type=type(error).__name__,
        exception_location=_exception_location(error),
    )


def _validate_application_error_map() -> None:
    missing = set(ERROR_CATEGORIES) - set(APPLICATION_ERRORS)
    extra = set(APPLICATION_ERRORS) - set(ERROR_CATEGORIES)
    if missing or extra:
        raise RuntimeError(f"application error map mismatch: missing={missing}, extra={extra}")
    for code, category in ERROR_CATEGORIES.items():
        response = APPLICATION_ERRORS[code]
        expected_status = {
            ErrorCategory.AUTHENTICATION: 401,
            ErrorCategory.AUTHORIZATION: 403,
            ErrorCategory.CONFLICT: 409,
            ErrorCategory.DEPENDENCY_UNAVAILABLE: 503,
            ErrorCategory.INVALID_INPUT: 422,
            ErrorCategory.NOT_FOUND: 404,
        }[category]
        if response.status_code != expected_status:
            raise RuntimeError(
                f"application error has invalid status: code={code}, expected={expected_status}"
            )


def register_exception_handlers(app: FastAPI) -> None:
    _validate_application_error_map()

    @app.exception_handler(ApplicationError)
    async def application_error_handler(
        request: Request,
        error: ApplicationError,
    ) -> JSONResponse:
        return _error_response(request, APPLICATION_ERRORS[error.code])

    @app.exception_handler(StarletteHTTPException)
    async def http_exception_handler(
        request: Request,
        error: StarletteHTTPException,
    ) -> JSONResponse:
        public_error = _http_error(error.status_code)
        return _error_response(request, public_error, headers=error.headers)

    @app.exception_handler(RequestValidationError)
    async def request_validation_error_handler(
        request: Request,
        error: RequestValidationError,
    ) -> JSONResponse:
        return _error_response(
            request,
            HTTP_ERRORS[422],
            details=_validation_details(error),
        )

    @app.exception_handler(ResponseValidationError)
    async def response_validation_error_handler(
        request: Request,
        error: ResponseValidationError,
    ) -> JSONResponse:
        _log_unexpected(request, error, "response_validation_failed")
        return _error_response(request, INTERNAL_ERROR)

    @app.exception_handler(Exception)
    async def unexpected_error_handler(request: Request, error: Exception) -> JSONResponse:
        _log_unexpected(request, error, "unhandled_exception")
        return _error_response(request, INTERNAL_ERROR)
