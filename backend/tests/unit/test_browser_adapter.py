from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from sources.adapters import browser_runtime
from sources.adapters.browser_runtime import BrowserRuntime, BrowserRuntimeDisabledError


def _playwright(monkeypatch: pytest.MonkeyPatch) -> tuple[MagicMock, AsyncMock, AsyncMock]:
    context = AsyncMock()
    browser = AsyncMock()
    browser.new_context.return_value = context
    playwright = MagicMock()
    playwright.chromium.connect = AsyncMock(return_value=browser)
    manager = MagicMock()
    manager.__aenter__ = AsyncMock(return_value=playwright)
    manager.__aexit__ = AsyncMock(return_value=None)
    monkeypatch.setattr(browser_runtime, "async_playwright", lambda: manager)
    return playwright, browser, context


def test_browser_runtime_rejects_unsafe_ws_urls() -> None:
    for url in (
        "https://browser:3000/",
        "ws://user:password@browser:3000/",
        "ws://browser:3000/other",
        "ws://browser:3000/?token=value",
        "ws://browser:3000/#fragment",
        "ws://browser/",
    ):
        with pytest.raises(ValueError):
            BrowserRuntime(ws_url=url, enabled=True)


def test_browser_runtime_is_disabled_by_default(monkeypatch: pytest.MonkeyPatch) -> None:
    _playwright(monkeypatch)
    runtime = BrowserRuntime(ws_url="ws://browser:3000/", enabled=False)

    async def run() -> None:
        with pytest.raises(BrowserRuntimeDisabledError):
            async with runtime.context():
                pass

    asyncio.run(run())


def test_browser_runtime_closes_context_and_connection_on_failure(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    playwright, browser, context = _playwright(monkeypatch)
    runtime = BrowserRuntime(ws_url="ws://browser:3000/", enabled=True, connect_timeout_ms=1500)

    async def run() -> None:
        with pytest.raises(RuntimeError, match="fixture failure"):
            async with runtime.context() as opened:
                assert opened is context
                raise RuntimeError("fixture failure")

    asyncio.run(run())

    playwright.chromium.connect.assert_awaited_once_with("ws://browser:3000/", timeout=1500)
    browser.new_context.assert_awaited_once_with(accept_downloads=False, service_workers="block")
    context.close.assert_awaited_once()
    browser.close.assert_awaited_once()


def test_browser_runtime_closes_after_cancel(monkeypatch: pytest.MonkeyPatch) -> None:
    _, browser, context = _playwright(monkeypatch)
    runtime = BrowserRuntime(ws_url="ws://browser:3000/", enabled=True)
    entered = asyncio.Event()

    async def hold_context() -> None:
        async with runtime.context():
            entered.set()
            await asyncio.Future()

    async def run() -> None:
        task = asyncio.create_task(hold_context())
        await entered.wait()
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task

    asyncio.run(run())

    context.close.assert_awaited_once()
    browser.close.assert_awaited_once()
