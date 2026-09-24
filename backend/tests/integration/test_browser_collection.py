"""Opt-in live checks against the isolated Playwright server."""

import asyncio
import multiprocessing
import os
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory
from threading import Event
from typing import Protocol
from uuid import uuid4

from playwright.async_api import Error as PlaywrightError
from playwright.async_api import async_playwright

from connections.adapters.local_secrets import BrowserStateStore
from sources.adapters.browser_runtime import BrowserRuntime
from worker.execution import JobProcessOutcome, JobProcessSupervisor

_FIXTURE_URL = "https://browser-fixture.invalid/"
_PAGE = """
<html>
  <body>
    <label>Query <input id="query"></label>
    <button id="submit">Apply</button>
    <p id="submitted"></p>
    <div style="height: 1200px"></div>
    <button id="expand">Expand</button>
    <p id="detail" hidden>Expanded detail</p>
    <p id="page">1</p>
    <button id="next">Next</button>
    <script>
      let currentPage = 1;
      document.querySelector('#submit').onclick = () => {
        document.querySelector('#submitted').textContent =
          document.querySelector('#query').value;
      };
      document.querySelector('#expand').onclick = () => {
        document.querySelector('#detail').hidden = false;
      };
      document.querySelector('#next').onclick = () => {
        currentPage += 1;
        document.querySelector('#page').textContent = String(currentPage);
        document.querySelector('#next').disabled = currentPage === 3;
      };
    </script>
  </body>
</html>
"""
_ENDLESS_PAGE = """
<button id="next">Next</button>
<p id="page">1</p>
<script>
  document.querySelector('#next').onclick = () => {
    const marker = document.querySelector('#page');
    marker.textContent = String(Number(marker.textContent) + 1);
  };
</script>
"""


class _ReadySignal(Protocol):
    def is_set(self) -> bool: ...

    def set(self) -> None: ...


def _hold_browser_context_until_cancelled(ready: _ReadySignal) -> None:
    runtime = BrowserRuntime(
        ws_url=os.environ.get("HOTKEY_BROWSER_WS_URL", ""),
        enabled=True,
    )

    async def hold_context() -> None:
        async with runtime.context() as context:
            page = await context.new_page()
            await page.set_content(_PAGE)
            ready.set()
            await asyncio.Future()

    asyncio.run(hold_context())


@unittest.skipUnless(
    os.environ.get("HOTKEY_BROWSER_LIVE_TEST") == "1",
    "requires the explicitly enabled isolated browser server",
)
class BrowserCollectionLiveTests(unittest.TestCase):
    def test_public_root_ws_path_is_rejected(self) -> None:
        async def verify() -> None:
            async with async_playwright() as playwright:
                with self.assertRaises(PlaywrightError):
                    await playwright.chromium.connect("ws://browser:3000/", timeout=1_500)

        asyncio.run(verify())

    @unittest.skipUnless(
        os.environ.get("HOTKEY_BROWSER_EGRESS_LIVE_TEST") == "1",
        "requires the dedicated browser egress proxy and a public canary",
    )
    def test_dedicated_proxy_allows_only_the_canary_and_blocks_loopback(self) -> None:
        async def verify() -> None:
            runtime = BrowserRuntime(
                ws_url=os.environ.get("HOTKEY_BROWSER_WS_URL", ""),
                enabled=True,
            )
            async with runtime.context() as context:
                page = await context.new_page()
                allowed = await page.goto("https://example.com/", timeout=10_000)
                self.assertIsNotNone(allowed)
                self.assertEqual(allowed.status, 200)

                for blocked_url in ("http://example.org/", "http://127.0.0.1:3000/"):
                    blocked = await page.goto(blocked_url, timeout=10_000)
                    self.assertIsNotNone(blocked)
                    self.assertEqual(blocked.status, 403)
                    self.assertIn("ERR_ACCESS_DENIED", blocked.headers["x-squid-error"])

        asyncio.run(verify())

    def test_dynamic_actions_have_a_bounded_end_and_contexts_are_isolated(self) -> None:
        async def verify() -> None:
            runtime = BrowserRuntime(
                ws_url=os.environ.get("HOTKEY_BROWSER_WS_URL", ""),
                enabled=True,
            )

            async with runtime.context() as first:
                await first.route(
                    _FIXTURE_URL,
                    lambda route: route.fulfill(status=200, body=_PAGE, content_type="text/html"),
                )
                page = await first.new_page()
                await page.goto(_FIXTURE_URL)
                await page.locator("#query").fill("bounded query")
                await page.locator("#submit").click()
                self.assertEqual(await page.locator("#submitted").inner_text(), "bounded query")

                await page.locator("#expand").scroll_into_view_if_needed()
                self.assertGreater(await page.evaluate("window.scrollY"), 0)
                await page.locator("#expand").click()
                self.assertTrue(await page.locator("#detail").is_visible())

                observed_pages: list[int] = []
                for _ in range(4):
                    observed_pages.append(int(await page.locator("#page").inner_text()))
                    if await page.locator("#next").is_disabled():
                        break
                    await page.locator("#next").click()
                else:
                    self.fail("pagination did not stop within the fixed limit")
                self.assertEqual(observed_pages, [1, 2, 3])

                await first.add_cookies(
                    [{"name": "session", "value": "fixture-only", "url": _FIXTURE_URL}]
                )
                await page.evaluate("localStorage.setItem('session', 'fixture-only')")
                state = await first.storage_state()

            with TemporaryDirectory() as directory:
                store = BrowserStateStore(Path(directory))
                owner_id, connection_id = uuid4(), uuid4()
                reference = store.save(
                    owner_id=owner_id,
                    connection_id=connection_id,
                    version=1,
                    state=state,
                )
                stored_state = store.load(
                    owner_id=owner_id,
                    connection_id=connection_id,
                    version=1,
                    reference=reference,
                )
                async with runtime.context(storage_state=stored_state) as authenticated:
                    await authenticated.route(
                        _FIXTURE_URL,
                        lambda route: route.fulfill(
                            status=200, body=_PAGE, content_type="text/html"
                        ),
                    )
                    page = await authenticated.new_page()
                    await page.goto(_FIXTURE_URL)
                    self.assertEqual(
                        (await authenticated.cookies(_FIXTURE_URL))[0]["value"], "fixture-only"
                    )
                    self.assertEqual(
                        await page.evaluate("localStorage.getItem('session')"), "fixture-only"
                    )

            async with runtime.context() as second:
                await second.route(
                    _FIXTURE_URL,
                    lambda route: route.fulfill(status=200, body=_PAGE, content_type="text/html"),
                )
                page = await second.new_page()
                await page.goto(_FIXTURE_URL)
                self.assertEqual(await second.cookies(_FIXTURE_URL), [])
                self.assertIsNone(await page.evaluate("localStorage.getItem('session')"))

        asyncio.run(verify())

    def test_endless_pagination_stops_at_the_explicit_limit(self) -> None:
        async def verify() -> None:
            runtime = BrowserRuntime(
                ws_url=os.environ.get("HOTKEY_BROWSER_WS_URL", ""),
                enabled=True,
            )
            async with runtime.context() as context:
                await context.route(
                    _FIXTURE_URL,
                    lambda route: route.fulfill(
                        status=200, body=_ENDLESS_PAGE, content_type="text/html"
                    ),
                )
                page = await context.new_page()
                await page.goto(_FIXTURE_URL)
                observed_pages: list[int] = []
                for _ in range(2):
                    observed_pages.append(int(await page.locator("#page").inner_text()))
                    if await page.locator("#next").is_disabled():
                        break
                    await page.locator("#next").click()
                self.assertEqual(observed_pages, [1, 2])
                self.assertEqual(await page.locator("#page").inner_text(), "3")
                self.assertTrue(await page.locator("#next").is_enabled())

        asyncio.run(verify())

    def test_timeout_and_cancellation_close_remote_contexts(self) -> None:
        async def verify() -> None:
            runtime = BrowserRuntime(
                ws_url=os.environ.get("HOTKEY_BROWSER_WS_URL", ""),
                enabled=True,
                execution_timeout_seconds=1,
            )
            timed_out_page = None
            with self.assertRaises(TimeoutError):
                async with runtime.context() as context:
                    timed_out_page = await context.new_page()
                    await timed_out_page.set_content("<main>bounded</main>")
                    await asyncio.sleep(2)
            self.assertIsNotNone(timed_out_page)
            assert timed_out_page is not None
            self.assertTrue(timed_out_page.is_closed())

            entered = asyncio.Event()
            cancelled_page = None

            async def hold_context() -> None:
                nonlocal cancelled_page
                async with runtime.context() as context:
                    cancelled_page = await context.new_page()
                    entered.set()
                    await asyncio.Future()

            task = asyncio.create_task(hold_context())
            await asyncio.wait_for(entered.wait(), timeout=5)
            task.cancel()
            with self.assertRaises(asyncio.CancelledError):
                await task
            self.assertIsNotNone(cancelled_page)
            assert cancelled_page is not None
            self.assertTrue(cancelled_page.is_closed())

            async with runtime.context() as context:
                page = await context.new_page()
                await page.set_content("<main>still available</main>")
                self.assertEqual(await page.locator("main").inner_text(), "still available")

        asyncio.run(verify())

    def test_supervisor_cancellation_releases_child_browser_context(self) -> None:
        ready = multiprocessing.get_context("spawn").Event()
        supervisor = JobProcessSupervisor(
            startup_timeout_seconds=5,
            execution_timeout_seconds=20,
            terminate_grace_seconds=1,
            poll_interval_seconds=0.05,
        )

        result = supervisor.run(
            _hold_browser_context_until_cancelled,
            (ready,),
            cancellation_requested=ready.is_set,
            stopping=Event(),
        )

        self.assertEqual(result.outcome, JobProcessOutcome.CANCELLED)

        async def verify_browser_remains_usable() -> None:
            runtime = BrowserRuntime(
                ws_url=os.environ.get("HOTKEY_BROWSER_WS_URL", ""),
                enabled=True,
            )
            async with runtime.context() as context:
                page = await context.new_page()
                await page.set_content("<main>browser recovered</main>")
                self.assertEqual(await page.locator("main").inner_text(), "browser recovered")

        asyncio.run(verify_browser_remains_usable())
