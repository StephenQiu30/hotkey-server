from __future__ import annotations

import asyncio
import math
import re
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from urllib.parse import urlsplit

from playwright.async_api import Browser, BrowserContext, StorageState, async_playwright

_CLOSE_TIMEOUT_SECONDS = 5


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
        if ws_url or enabled:
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
                or re.fullmatch(r"/ws/[0-9a-f]{48}", parsed.path) is None
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
        manager = async_playwright()
        browser: Browser | None = None
        context: BrowserContext | None = None
        try:
            async with asyncio.timeout(self._execution_timeout_seconds):
                playwright = await manager.__aenter__()
                browser = await playwright.chromium.connect(
                    self._ws_url, timeout=self._connect_timeout_ms
                )
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
                yield context
        finally:
            try:
                try:
                    if context is not None:
                        await asyncio.wait_for(context.close(), timeout=_CLOSE_TIMEOUT_SECONDS)
                finally:
                    if browser is not None:
                        await asyncio.wait_for(browser.close(), timeout=_CLOSE_TIMEOUT_SECONDS)
            finally:
                await asyncio.wait_for(
                    manager.__aexit__(None, None, None), timeout=_CLOSE_TIMEOUT_SECONDS
                )
