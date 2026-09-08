from uuid import uuid4

from fastapi.responses import JSONResponse
from starlette.types import ASGIApp, Message, Receive, Scope, Send


class Boundary:
    """Bound bodies before parsing and validate browser origins for unsafe requests."""

    def __init__(self, app: ASGIApp):
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        request_id = str(uuid4())
        scope.setdefault("state", {})["request_id"] = request_id
        headers = dict(scope["headers"])

        async def respond(code: str, status: int) -> None:
            response = JSONResponse(
                {"code": code, "request_id": request_id},
                status,
                headers={"X-Request-ID": request_id, "Cache-Control": "no-store"},
            )
            await response(scope, receive, send)

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

        async def tracked(message: Message) -> None:
            nonlocal started
            if message["type"] == "http.response.start":
                started = True
            await tagged(message)

        try:
            await self.app(scope, replay, tracked)
        except Exception as exc:
            import logging

            logging.error("request_failed request_id=%s type=%s", request_id, type(exc).__name__)
            if started:
                raise
            await respond("internal_error", 500)
