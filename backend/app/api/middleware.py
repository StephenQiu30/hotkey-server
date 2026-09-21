from __future__ import annotations

from time import perf_counter
from uuid import uuid4

import structlog
from fastapi import FastAPI
from starlette.types import ASGIApp, Message, Receive, Scope, Send
from structlog.contextvars import bind_contextvars, clear_contextvars


class RequestContextMiddleware:
    def __init__(self, app: ASGIApp) -> None:
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        request_id = str(uuid4())
        state = scope.setdefault("state", {})
        state["request_id"] = request_id
        status_code = 500
        started_at = perf_counter()
        clear_contextvars()
        bind_contextvars(request_id=request_id)

        async def send_with_context(message: Message) -> None:
            nonlocal status_code
            if message["type"] == "http.response.start":
                status_code = message["status"]
                headers = [
                    (name, value)
                    for name, value in message.get("headers", [])
                    if name.lower() != b"x-request-id"
                ]
                headers.append((b"x-request-id", request_id.encode("ascii")))
                message["headers"] = headers
            await send(message)

        try:
            await self.app(scope, receive, send_with_context)
        finally:
            route = scope.get("route")
            route_path = getattr(route, "path", None)
            route_template = (
                f"{scope.get('root_path', '')}{route_path}" if route_path else "unmatched"
            )
            structlog.get_logger("http").info(
                "request_completed",
                method=scope.get("method", "UNKNOWN"),
                route=route_template,
                status_code=status_code,
                duration_ms=round((perf_counter() - started_at) * 1000, 2),
            )
            clear_contextvars()


def register_middleware(app: FastAPI) -> None:
    app.add_middleware(RequestContextMiddleware)
