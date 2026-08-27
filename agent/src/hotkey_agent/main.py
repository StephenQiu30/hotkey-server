from __future__ import annotations

import asyncio
import hmac

from fastapi import FastAPI, Header, Request, status
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.types import ASGIApp, Message, Receive, Scope, Send

from hotkey_agent import __version__
from hotkey_agent.analyzer import DeterministicAnalyzer
from hotkey_agent.config import Settings
from hotkey_agent.contracts import (
    AnalyzeRequest,
    AnalyzeResponse,
    ErrorBody,
    ErrorResponse,
    HealthResponse,
)


class RequestTooLargeError(Exception):
    pass


class RequestSizeMiddleware:
    def __init__(self, app: ASGIApp, max_bytes: int) -> None:
        self.app = app
        self.max_bytes = max_bytes

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        content_length = next(
            (value for key, value in scope["headers"] if key.lower() == b"content-length"),
            b"",
        )
        if content_length.isdigit() and int(content_length) > self.max_bytes:
            await _error(
                status.HTTP_413_CONTENT_TOO_LARGE,
                "AGENT_REQUEST_TOO_LARGE",
                "request exceeds configured limit",
            )(scope, receive, send)
            return
        consumed = 0

        async def receive_limited() -> Message:
            nonlocal consumed
            message = await receive()
            if message["type"] == "http.request":
                consumed += len(message.get("body", b""))
                if consumed > self.max_bytes:
                    raise RequestTooLargeError
            return message

        try:
            await self.app(scope, receive_limited, send)
        except RequestTooLargeError:
            await _error(
                status.HTTP_413_CONTENT_TOO_LARGE,
                "AGENT_REQUEST_TOO_LARGE",
                "request exceeds configured limit",
            )(scope, receive, send)


def create_app(settings: Settings | None = None) -> FastAPI:
    service_settings = settings or Settings.from_env()
    analyzer = DeterministicAnalyzer()
    capacity = asyncio.Semaphore(service_settings.max_concurrency)
    application = FastAPI(
        title="HotKey Internal Agent",
        version=__version__,
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
    )
    application.add_middleware(RequestSizeMiddleware, max_bytes=service_settings.max_request_bytes)

    @application.exception_handler(RequestValidationError)
    async def validation_error(
        _request: Request, _error_value: RequestValidationError
    ) -> JSONResponse:
        return _error(
            status.HTTP_422_UNPROCESSABLE_CONTENT,
            "AGENT_INVALID_REQUEST",
            "request does not match analysis.v1",
        )

    @application.get("/healthz", response_model=HealthResponse)
    async def health() -> HealthResponse:
        return HealthResponse(status="ok", version=__version__)

    @application.get("/readyz", response_model=HealthResponse)
    async def ready() -> HealthResponse | JSONResponse:
        if not service_settings.ready:
            return JSONResponse(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                content=HealthResponse(status="not_ready", version=__version__).model_dump(),
            )
        return HealthResponse(status="ok", version=__version__)

    @application.post("/v1/analyze", response_model=AnalyzeResponse)
    async def analyze(
        request: AnalyzeRequest,
        agent_token: str | None = Header(default=None, alias="X-HotKey-Agent-Token"),
    ) -> AnalyzeResponse | JSONResponse:
        if not service_settings.ready:
            return _error(
                status.HTTP_503_SERVICE_UNAVAILABLE,
                "AGENT_NOT_READY",
                "analysis runtime is not ready",
            )
        if agent_token is None or not hmac.compare_digest(agent_token, service_settings.auth_token):
            return _error(
                status.HTTP_401_UNAUTHORIZED, "AGENT_UNAUTHORIZED", "invalid service credential"
            )
        async with capacity:
            return await analyzer.analyze(request)

    return application


def _error(status_code: int, code: str, message: str) -> JSONResponse:
    body = ErrorResponse(error=ErrorBody(code=code, message=message))
    return JSONResponse(status_code=status_code, content=body.model_dump())


app = create_app()
