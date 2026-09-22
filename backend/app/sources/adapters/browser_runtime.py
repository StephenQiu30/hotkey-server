from __future__ import annotations

import asyncio
import math
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from urllib.parse import urlsplit

from playwright.async_api import BrowserContext, StorageState, async_playwright


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
        execution_timeout_seconds: float = 45,
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
        if not math.isfinite(execution_timeout_seconds) or not 0 < execution_timeout_seconds <= 45:
            raise ValueError("invalid browser execution timeout")
        self._ws_url = ws_url
        self._enabled = enabled
        self._connect_timeout_ms = connect_timeout_ms
        self._execution_timeout_seconds = execution_timeout_seconds

    @asynccontextmanager
    async def context(
        self, *, storage_state: StorageState | None = None
    ) -> AsyncIterator[BrowserContext]:
        if not self._enabled:
            raise BrowserRuntimeDisabledError
        async with async_playwright() as playwright:
            browser = await playwright.chromium.connect(
                self._ws_url, timeout=self._connect_timeout_ms
            )
            try:
                if storage_state is None:
                    context = await browser.new_context(
                        accept_downloads=False, service_workers="block"
                    )
                else:
                    context = await browser.new_context(
                        accept_downloads=False,
                        service_workers="block",
                        storage_state=storage_state,
                    )
                try:
                    async with asyncio.timeout(self._execution_timeout_seconds):
                        yield context
                finally:
                    await context.close()
            finally:
                await browser.close()
