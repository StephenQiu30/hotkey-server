import logging
from time import perf_counter
from uuid import uuid4

from fastapi.responses import JSONResponse
from starlette.types import ASGIApp, Message, Receive, Scope, Send

logger = logging.getLogger(__name__)


class Boundary:
    """Bound bodies before parsing and validate browser origins for unsafe requests."""

    def __init__(self, app: ASGIApp):
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        request_id = str(uuid4())
        started_at = perf_counter()
        scope.setdefault("state", {})["request_id"] = request_id
        headers = dict(scope["headers"])

        def log_completed(status_code: int) -> None:
            route = getattr(scope.get("route"), "path", "<unmatched>")
            duration_ms = round((perf_counter() - started_at) * 1000, 3)
            logger.info(
                "http_request_completed request_id=%s method=%s route=%s "
                "status_code=%s duration_ms=%s",
                request_id,
                scope["method"],
                route,
                status_code,
                duration_ms,
            )

        async def respond(code: str, status: int) -> None:
            response = JSONResponse(
                {"code": code, "request_id": request_id},
                status,
                headers={"X-Request-ID": request_id, "Cache-Control": "no-store"},
            )
            await response(scope, receive, send)
            log_completed(status)

        async def tagged(message: Message) -> None:
            if message["type"] == "http.response.start":
                message.setdefault("headers", []).extend(
                    [
                        (b"x-request-id", request_id.encode()),
                        (b"cache-control", b"no-store"),
                        (b"x-content-type-options", b"nosniff"),
                    ]
                )
            await send(message)

        if scope["method"] not in {"GET", "HEAD", "OPTIONS"}:
            origin = headers.get(b"origin", b"").decode("latin1")
            if origin not in scope["app"].state.settings.allowed_origins:
                await respond("origin_forbidden", 403)
                return
        parts = []
        size = 0
        while True:
            message = await receive()
            if message["type"] == "http.disconnect":
                return
            size += len(message.get("body", b""))
            if size > 65536:
                await respond("request_too_large", 413)
                return
            parts.append(message.get("body", b""))
            if not message.get("more_body", False):
                break
        consumed = False

        async def replay() -> Message:
            nonlocal consumed
            if not consumed:
                consumed = True
                return {"type": "http.request", "body": b"".join(parts), "more_body": False}
            return await receive()

        started = False
        status_code = 500

        async def tracked(message: Message) -> None:
            nonlocal started, status_code
            if message["type"] == "http.response.start":
                started = True
                status_code = message["status"]
            await tagged(message)

        try:
            await self.app(scope, replay, tracked)
            log_completed(status_code)
        except Exception as exc:
            logger.exception(
                "http_request_failed request_id=%s method=%s type=%s",
                request_id,
                scope["method"],
                type(exc).__name__,
            )
            if started:
                raise
            await respond("internal_error", 500)
