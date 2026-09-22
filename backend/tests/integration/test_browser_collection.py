"""Opt-in live checks against the isolated Playwright server."""

import asyncio
import os
import unittest

from sources.adapters.browser_runtime import BrowserRuntime

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


@unittest.skipUnless(
    os.environ.get("HOTKEY_BROWSER_LIVE_TEST") == "1",
    "requires the explicitly enabled isolated browser server",
)
class BrowserCollectionLiveTests(unittest.TestCase):
    def test_dynamic_actions_have_a_bounded_end_and_contexts_are_isolated(self) -> None:
        async def verify() -> None:
            runtime = BrowserRuntime(
                ws_url=os.environ.get("HOTKEY_BROWSER_WS_URL", "ws://browser:3000/"),
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
                ws_url=os.environ.get("HOTKEY_BROWSER_WS_URL", "ws://browser:3000/"),
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
