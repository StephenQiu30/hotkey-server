from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from urllib.parse import urlsplit

from playwright.async_api import BrowserContext, async_playwright


class BrowserRuntimeDisabledError(Exception):
    """The optional browser runtime is disabled."""


class BrowserRuntime:
    """Connect to the managed Playwright server for one isolated context."""

    def __init__(
        self,
        *,
        ws_url: str,
        enabled: bool,
        connect_timeout_ms: int = 5_000,
    ) -> None:
        try:
            parsed = urlsplit(ws_url)
            port = parsed.port
        except ValueError as error:
            raise ValueError("invalid browser WS URL") from error
        if (
            parsed.scheme not in {"ws", "wss"}
            or parsed.hostname is None
            or parsed.username is not None
            or parsed.password is not None
            or parsed.path not in {"", "/"}
            or parsed.query
            or parsed.fragment
            or port is None
        ):
            raise ValueError("invalid browser WS URL")
        if not 1 <= connect_timeout_ms <= 10_000:
            raise ValueError("invalid browser connect timeout")
        self._ws_url = ws_url
        self._enabled = enabled
        self._connect_timeout_ms = connect_timeout_ms

    @asynccontextmanager
    async def context(self) -> AsyncIterator[BrowserContext]:
        if not self._enabled:
            raise BrowserRuntimeDisabledError
        async with async_playwright() as playwright:
            browser = await playwright.chromium.connect(
                self._ws_url, timeout=self._connect_timeout_ms
            )
            try:
                context = await browser.new_context(
                    accept_downloads=False, service_workers="block"
                )
                try:
                    yield context
                finally:
                    await context.close()
            finally:
                await browser.close()
