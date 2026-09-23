from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from sources.adapters import browser_runtime
from sources.adapters.browser_runtime import BrowserRuntime, BrowserRuntimeDisabledError

_WS_URL = "ws://browser:3000/ws/" + "a" * 48


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
        "ws://browser:3000/",
        "https://browser:3000/",
        "ws://user:password@browser:3000/",
        "ws://browser:3000/other",
        "ws://browser:3000/ws/short",
        "ws://browser:3000/?token=value",
        "ws://browser:3000/#fragment",
        "ws://browser/",
    ):
        with pytest.raises(ValueError):
            BrowserRuntime(ws_url=url, enabled=True)


def test_browser_runtime_is_disabled_by_default(monkeypatch: pytest.MonkeyPatch) -> None:
    _playwright(monkeypatch)
    runtime = BrowserRuntime(ws_url="", enabled=False)

    async def run() -> None:
        with pytest.raises(BrowserRuntimeDisabledError):
            async with runtime.context():
                pass

    asyncio.run(run())


def test_browser_runtime_closes_context_and_connection_on_failure(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    playwright, browser, context = _playwright(monkeypatch)
    runtime = BrowserRuntime(ws_url=_WS_URL, enabled=True, connect_timeout_ms=1500)

    async def run() -> None:
        with pytest.raises(RuntimeError, match="fixture failure"):
            async with runtime.context() as opened:
                assert opened is context
                raise RuntimeError("fixture failure")

    asyncio.run(run())

    playwright.chromium.connect.assert_awaited_once_with(_WS_URL, timeout=1500)
    browser.new_context.assert_awaited_once_with(accept_downloads=False, service_workers="block")
    context.close.assert_awaited_once()
    browser.close.assert_awaited_once()


def test_browser_runtime_preserves_operation_error_when_context_cleanup_times_out(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _, browser, context = _playwright(monkeypatch)

    async def slow_close() -> None:
        await asyncio.sleep(1)

    context.close.side_effect = slow_close
    monkeypatch.setattr(browser_runtime, "_CLOSE_TIMEOUT_SECONDS", 0.01)
    runtime = BrowserRuntime(ws_url=_WS_URL, enabled=True)

    async def run() -> None:
        with pytest.raises(RuntimeError, match="source operation failed") as captured:
            async with runtime.context():
                raise RuntimeError("source operation failed")
        assert any("TimeoutError" in note for note in captured.value.__notes__)

    asyncio.run(run())

    context.close.assert_awaited_once()
    browser.close.assert_awaited_once()


def test_browser_runtime_closes_after_cancel(monkeypatch: pytest.MonkeyPatch) -> None:
    _, browser, context = _playwright(monkeypatch)
    runtime = BrowserRuntime(ws_url=_WS_URL, enabled=True)
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


def test_browser_runtime_loads_only_an_explicit_state_object(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _, browser, _ = _playwright(monkeypatch)
    runtime = BrowserRuntime(ws_url=_WS_URL, enabled=True)
    state = {"cookies": [], "origins": []}

    async def run() -> None:
        async with runtime.context(storage_state=state):
            pass

    asyncio.run(run())
    browser.new_context.assert_awaited_once_with(
        accept_downloads=False, service_workers="block", storage_state=state
    )


def test_browser_runtime_enforces_interaction_deadline_and_closes(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _, browser, context = _playwright(monkeypatch)
    runtime = BrowserRuntime(
        ws_url=_WS_URL,
        enabled=True,
        execution_timeout_seconds=0.01,
    )

    async def run() -> None:
        with pytest.raises(TimeoutError):
            async with runtime.context():
                await asyncio.sleep(1)

    asyncio.run(run())
    context.close.assert_awaited_once()
    browser.close.assert_awaited_once()


def test_browser_runtime_deadline_includes_context_creation(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _, browser, context = _playwright(monkeypatch)

    async def slow_context(**_kwargs: object) -> AsyncMock:
        await asyncio.sleep(1)
        return context

    browser.new_context.side_effect = slow_context
    runtime = BrowserRuntime(
        ws_url=_WS_URL,
        enabled=True,
        execution_timeout_seconds=0.01,
    )

    async def run() -> None:
        with pytest.raises(TimeoutError):
            async with runtime.context():
                pass

    asyncio.run(run())
    context.close.assert_not_awaited()
    browser.close.assert_awaited_once()


def test_browser_runtime_deadline_includes_ws_connection(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    playwright, browser, _ = _playwright(monkeypatch)

    async def slow_connect(*_args: object, **_kwargs: object) -> AsyncMock:
        await asyncio.sleep(1)
        return browser

    playwright.chromium.connect.side_effect = slow_connect
    runtime = BrowserRuntime(
        ws_url=_WS_URL,
        enabled=True,
        execution_timeout_seconds=0.01,
    )

    async def run() -> None:
        with pytest.raises(TimeoutError):
            async with runtime.context():
                pass

    asyncio.run(run())
    browser.new_context.assert_not_awaited()
    browser.close.assert_not_awaited()


def test_browser_runtime_deadline_includes_manager_startup(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    manager = MagicMock()

    async def slow_startup() -> MagicMock:
        await asyncio.sleep(0.05)
        playwright = MagicMock()
        playwright.chromium.connect = AsyncMock()
        return playwright

    manager.__aenter__ = AsyncMock(side_effect=slow_startup)
    manager.__aexit__ = AsyncMock(return_value=None)
    monkeypatch.setattr(browser_runtime, "async_playwright", lambda: manager)
    runtime = BrowserRuntime(ws_url=_WS_URL, enabled=True, execution_timeout_seconds=0.01)

    async def run() -> None:
        with pytest.raises(TimeoutError):
            async with runtime.context():
                pass

    asyncio.run(run())
    manager.__aexit__.assert_awaited_once()


def test_browser_runtime_bounds_manager_shutdown(monkeypatch: pytest.MonkeyPatch) -> None:
    playwright, browser, context = _playwright(monkeypatch)
    manager = MagicMock()
    manager.__aenter__ = AsyncMock(return_value=playwright)

    async def slow_shutdown(*_args: object) -> None:
        await asyncio.sleep(0.05)

    manager.__aexit__ = AsyncMock(side_effect=slow_shutdown)
    monkeypatch.setattr(browser_runtime, "async_playwright", lambda: manager)
    monkeypatch.setattr(browser_runtime, "_CLOSE_TIMEOUT_SECONDS", 0.01)
    runtime = BrowserRuntime(ws_url=_WS_URL, enabled=True)

    async def run() -> None:
        with pytest.raises(TimeoutError):
            async with runtime.context():
                pass

    asyncio.run(run())
    context.close.assert_awaited_once()
    browser.close.assert_awaited_once()
    manager.__aexit__.assert_awaited_once()


def test_browser_runtime_attempts_disconnect_after_slow_context_close(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _, browser, context = _playwright(monkeypatch)

    async def slow_close() -> None:
        await asyncio.sleep(1)

    context.close.side_effect = slow_close
    monkeypatch.setattr(browser_runtime, "_CLOSE_TIMEOUT_SECONDS", 0.01, raising=False)
    runtime = BrowserRuntime(ws_url=_WS_URL, enabled=True)

    async def run() -> None:
        with pytest.raises(TimeoutError):
            async with runtime.context():
                pass

    asyncio.run(run())
    context.close.assert_awaited_once()
    browser.close.assert_awaited_once()


def test_browser_runtime_bounds_slow_disconnect(monkeypatch: pytest.MonkeyPatch) -> None:
    _, browser, context = _playwright(monkeypatch)

    async def slow_close() -> None:
        await asyncio.sleep(1)

    browser.close.side_effect = slow_close
    monkeypatch.setattr(browser_runtime, "_CLOSE_TIMEOUT_SECONDS", 0.01)
    runtime = BrowserRuntime(ws_url=_WS_URL, enabled=True)

    async def run() -> None:
        with pytest.raises(TimeoutError):
            async with runtime.context():
                pass

    asyncio.run(run())
    context.close.assert_awaited_once()
    browser.close.assert_awaited_once()


def test_browser_runtime_rejects_unbounded_interaction_deadline() -> None:
    for duration in (0, -1, 46, float("nan"), float("inf")):
        with pytest.raises(ValueError, match="execution timeout"):
            BrowserRuntime(
                ws_url=_WS_URL,
                enabled=True,
                execution_timeout_seconds=duration,
            )
