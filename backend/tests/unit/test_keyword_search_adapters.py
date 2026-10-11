from __future__ import annotations

from datetime import UTC, datetime, timedelta
from types import SimpleNamespace
from uuid import uuid4

import httpx
import pytest
from sqlalchemy.orm import sessionmaker

import content.discovery_execution as discovery_execution
from connections.schemas import SourceConnectionConfig
from content.discovery_execution import (
    SearchRequestGuardUnavailableError,
    UnsupportedSearchSourceError,
    build_search_adapter_factory,
)
from core.config import Settings
from sources.adapters.hackernews import HackerNewsAdapter
from sources.adapters.rss import RssSourceAdapter
from sources.adapters.web_search import WebSearchAdapter
from sources.contracts import (
    SearchRequest,
    SourcePageState,
    SourcePost,
    SourceSort,
    SourceStopReason,
)
from worker.app import _registered_job_handlers


def _factory(source_key: str, **config: object):
    return build_search_adapter_factory(
        source_key,
        SourceConnectionConfig.model_validate(config),
    )(
        lambda _attempt: True,
        lambda: False,
        4,
        30.0,
    )


def test_hackernews_factory_uses_connection_allowed_hosts() -> None:
    adapter = _factory(
        "hackernews",
        base_url="https://hn.algolia.com/api/v1",
        allowed_hosts=("hn.algolia.com",),
    )

    assert isinstance(adapter, HackerNewsAdapter)
    assert adapter.source_key == "hackernews"
    assert adapter._api == "https://hn.algolia.com/api/v1"
    assert adapter._allowed_hosts == frozenset({"hn.algolia.com"})


def _hn_search_request(*, page_size: int = 2, page_token: str | None = None) -> SearchRequest:
    return SearchRequest(
        source_key="hackernews",
        query="AI",
        sort=SourceSort.LATEST,
        page_size=page_size,
        page_token=page_token,
        starts_at=datetime(2026, 9, 25, tzinfo=UTC),
        ends_at=datetime(2026, 9, 26, tzinfo=UTC),
    )


def test_hn_search_keeps_opaque_object_ids_and_nullable_story_fields() -> None:
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(
            200,
            json={
                "page": 0,
                "hitsPerPage": 2,
                "nbPages": 1,
                "nbHits": 2,
                "exhaustiveNbHits": True,
                "hits": [
                    {
                        "objectID": "000123",
                        "title": "Same title",
                        "story_text": "<p>First <b>body</b></p>",
                        "url": "https://example.com/a",
                        "author": "alice",
                        "created_at_i": 1790323200,
                        "points": 0,
                        "num_comments": 7,
                    },
                    {
                        "objectID": "story:a/b",
                        "title": "Same title",
                        "url": "https://example.com/b",
                        "author": None,
                        "created_at_i": None,
                        "points": None,
                        "num_comments": "unknown",
                    },
                ],
            },
        )

    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True, transport=httpx.MockTransport(handler)
    )
    page = adapter.fetch_page(_hn_search_request())

    assert page.state is SourcePageState.COMPLETE
    assert page.request_count == 1
    assert [item.external_id for item in page.items] == ["000123", "story:a/b"]
    first, second = page.items
    assert isinstance(first, SourcePost) and isinstance(second, SourcePost)
    assert first.canonical_url == "https://news.ycombinator.com/item?id=000123"
    assert first.text == "First body"
    assert first.author_external_id == first.author_name == "alice"
    assert first.published_at == datetime.fromtimestamp(1790323200, UTC)
    assert first.like_count == 0 and first.comment_count == 7
    assert second.canonical_url == "https://news.ycombinator.com/item?id=story%3Aa%2Fb"
    assert second.text is None and second.published_at is None
    assert second.like_count is None and second.comment_count is None
    assert seen[0].url.path == "/api/v1/search_by_date"
    assert seen[0].url.params["tags"] == "story"
    assert page.terminal_evidence is not None
    assert page.terminal_evidence.terminal_verified is True
    assert page.terminal_evidence.query_bounded is True
    assert page.terminal_evidence.sort_applied is True
    assert page.terminal_evidence.sort_key is SourceSort.LATEST


def test_hn_search_uses_two_pages_and_proves_only_the_true_tail() -> None:
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        page = int(request.url.params["page"])
        return httpx.Response(
            200,
            json={
                "page": page,
                "hitsPerPage": 1,
                "nbPages": 2,
                "nbHits": 2,
                "exhaustiveNbHits": True,
                "hits": [{"objectID": f"{page + 100}", "title": f"Story {page}"}],
            },
        )

    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True, transport=httpx.MockTransport(handler)
    )
    first = adapter.fetch_page(_hn_search_request(page_size=1))
    second = adapter.fetch_page(_hn_search_request(page_size=1, page_token=first.next_page_token))

    assert first.state is SourcePageState.MORE and first.next_page_token == "1"
    assert first.terminal_evidence is None
    assert second.state is SourcePageState.COMPLETE and second.next_page_token is None
    assert second.terminal_evidence is not None
    assert second.terminal_evidence.terminal_verified is True
    assert [request.url.params["page"] for request in seen] == ["0", "1"]


def test_hn_second_page_failure_keeps_first_page_without_terminal_proof() -> None:
    seen: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        page = request.url.params["page"]
        seen.append(page)
        if page == "1":
            return httpx.Response(503)
        return httpx.Response(
            200,
            json={
                "page": 0,
                "hitsPerPage": 1,
                "nbPages": 2,
                "nbHits": 2,
                "exhaustiveNbHits": True,
                "hits": [{"objectID": "first-story", "title": "AI first story"}],
            },
        )

    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True, transport=httpx.MockTransport(handler)
    )
    first = adapter.fetch_page(_hn_search_request(page_size=1))
    second = adapter.fetch_page(_hn_search_request(page_size=1, page_token=first.next_page_token))

    assert first.state is SourcePageState.MORE
    assert first.terminal_evidence is None
    assert len(first.items) == 1
    assert second.state is SourcePageState.STOPPED
    assert second.stop_reason is SourceStopReason.UPSTREAM_ERROR
    assert second.terminal_evidence is None
    assert seen == ["0", "1"]


def test_hn_budget_refusal_sends_no_external_request() -> None:
    sent: list[httpx.Request] = []
    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: False,
        transport=httpx.MockTransport(
            lambda request: sent.append(request) or httpx.Response(200, json={"hits": []})
        ),
    )

    page = adapter.fetch_page(_hn_search_request())
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert sent == []


@pytest.mark.parametrize("bad_pages", [True, "2", -1])
def test_hn_search_rejects_malformed_page_metadata(bad_pages: object) -> None:
    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(
                200,
                json={"hits": [{"objectID": "123", "title": "Story"}], "nbPages": bad_pages},
            )
        ),
    )
    page = adapter.fetch_page(_hn_search_request(page_size=1))
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR


@pytest.mark.parametrize(
    "metadata",
    [
        {"page": 1, "hitsPerPage": 1, "nbPages": 2},
        {"page": 0, "hitsPerPage": 2, "nbPages": 1},
    ],
)
def test_hn_search_rejects_mismatched_page_metadata(metadata: dict[str, object]) -> None:
    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(
                200,
                json={"hits": [{"objectID": "123", "title": "Story"}], **metadata},
            )
        ),
    )
    page = adapter.fetch_page(_hn_search_request(page_size=1))
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR


@pytest.mark.parametrize(
    "metadata",
    [
        {"nbPages": 1},  # missing total/page-size metadata
        {"page": 0, "hitsPerPage": 1, "nbPages": 1, "nbHits": 2, "exhaustiveNbHits": True},
        {"page": 0, "hitsPerPage": 1, "nbPages": 1, "nbHits": 1, "exhaustiveNbHits": False},
    ],
)
def test_hn_search_keeps_uncertain_terminal_metadata_unverified(
    metadata: dict[str, object],
) -> None:
    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(
                200, json={"hits": [{"objectID": "123", "title": "Story"}], **metadata}
            )
        ),
    )
    page = adapter.fetch_page(_hn_search_request(page_size=1))
    assert page.state is SourcePageState.COMPLETE
    assert page.terminal_evidence is not None
    assert page.terminal_evidence.terminal_verified is False


def test_hn_search_caps_page_count_without_claiming_terminal_proof() -> None:
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(
            200,
            json={
                "page": 49,
                "hitsPerPage": 1,
                "nbPages": 51,
                "nbHits": 51,
                "exhaustiveNbHits": True,
                "hits": [{"objectID": "123", "title": "Story"}],
            },
        )

    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True, transport=httpx.MockTransport(handler)
    )
    page = adapter.fetch_page(_hn_search_request(page_size=1, page_token="49"))
    assert len(seen) == 1
    assert page.state is SourcePageState.COMPLETE
    assert page.terminal_evidence is not None
    assert page.terminal_evidence.terminal_verified is False


def test_hn_search_does_not_verify_an_out_of_range_page() -> None:
    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(
                200,
                json={
                    "page": 1,
                    "hitsPerPage": 1,
                    "nbPages": 1,
                    "nbHits": 1,
                    "exhaustiveNbHits": True,
                    "hits": [],
                },
            )
        ),
    )
    page = adapter.fetch_page(_hn_search_request(page_size=1, page_token="1"))
    assert page.state is SourcePageState.EMPTY
    assert page.terminal_evidence is not None
    assert page.terminal_evidence.terminal_verified is False


def test_hn_search_rejects_unrepresentable_timestamp_as_protocol_error() -> None:
    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(
                200,
                json={
                    "nbPages": 1,
                    "hits": [{"objectID": "123", "created_at_i": 10**100}],
                },
            )
        ),
    )
    page = adapter.fetch_page(_hn_search_request(page_size=1))
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.PROTOCOL_ERROR


def test_hn_search_ceil_seconds_preserves_fractional_utc_window() -> None:
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(200, json={"hits": [], "nbPages": 0})

    starts_at = datetime(2026, 9, 25, tzinfo=UTC) + timedelta(microseconds=500_000)
    ends_at = starts_at + timedelta(hours=24)
    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True, transport=httpx.MockTransport(handler)
    )
    adapter.fetch_page(
        _hn_search_request().model_copy(update={"starts_at": starts_at, "ends_at": ends_at})
    )
    assert seen[0].url.params["numericFilters"] == (
        f"created_at_i>={int(starts_at.timestamp()) + 1},"
        f"created_at_i<{int(ends_at.timestamp()) + 1}"
    )


@pytest.mark.parametrize(
    "base_url,allowed_hosts",
    [
        ("http://hn.algolia.com/api/v1", frozenset({"hn.algolia.com"})),
        ("https://other.example/api/v1", frozenset({"other.example"})),
        ("https://hn.algolia.com/api/v1/other", frozenset({"hn.algolia.com"})),
        ("https://hn.algolia.com:8443/api/v1", frozenset({"hn.algolia.com"})),
    ],
)
def test_hn_search_requires_fixed_https_endpoint(
    base_url: str, allowed_hosts: frozenset[str]
) -> None:
    with pytest.raises(ValueError, match="Algolia API"):
        HackerNewsAdapter(
            before_request=lambda _attempt: True,
            base_url=base_url,
            allowed_hosts=allowed_hosts,
        )


@pytest.mark.parametrize(
    "redirect_to",
    [
        "https://evil.example/api/v1/search_by_date",
        "http://hn.algolia.com/api/v1/search_by_date",
        "https://hn.algolia.com/api/v1/admin",
        "https://hn.algolia.com/api/v1/search_by_date",
    ],
)
def test_hn_search_rejects_redirect_outside_fixed_endpoints(redirect_to: str) -> None:
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(302, headers={"location": redirect_to})

    adapter = HackerNewsAdapter(
        before_request=lambda _attempt: True,
        allowed_hosts=frozenset({"hn.algolia.com", "evil.example"}),
        transport=httpx.MockTransport(handler),
    )
    page = adapter.fetch_page(_hn_search_request())
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.ACCESS_DENIED
    assert len(seen) == 1


def test_google_news_factory_requires_fixed_feed_template() -> None:
    adapter = _factory(
        "google_news",
        feed_url_template=(
            "https://news.google.com/rss/search?q={query}&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"
        ),
        allowed_hosts=("news.google.com",),
    )

    assert isinstance(adapter, RssSourceAdapter)
    assert adapter.source_key == "google_news"
    assert adapter._template == (
        "https://news.google.com/rss/search?q={query}&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"
    )
    with pytest.raises(ValueError, match="Google News"):
        _factory(
            "google_news",
            feed_url_template="https://news.google.com/rss/search?q={query}",
            allowed_hosts=("news.google.com",),
        )


def test_36kr_factory_requires_newsflashes_on_local_rsshub() -> None:
    adapter = _factory(
        "rss_36kr",
        feed_url_template="http://127.0.0.1:1200/36kr/newsflashes",
        allowed_hosts=("127.0.0.1",),
    )
    assert isinstance(adapter, RssSourceAdapter)
    assert adapter._template == "http://127.0.0.1:1200/36kr/newsflashes"
    docker_adapter = _factory(
        "rss_36kr",
        feed_url_template="http://host.docker.internal:1200/36kr/newsflashes",
        allowed_hosts=("host.docker.internal",),
    )
    assert docker_adapter._template == "http://host.docker.internal:1200/36kr/newsflashes"
    assert not docker_adapter._follow_redirects()

    with pytest.raises(ValueError, match="newsflashes"):
        _factory(
            "rss_36kr",
            feed_url_template="http://127.0.0.1:1200/36kr/hot-list",
            allowed_hosts=("127.0.0.1",),
        )
    with pytest.raises(ValueError, match="local"):
        _factory(
            "rss_36kr",
            feed_url_template="https://36kr.com/feed",
            allowed_hosts=("36kr.com",),
        )


@pytest.mark.parametrize("host", ["127.0.0.1", "host.docker.internal"])
def test_web_search_factory_requires_local_fixed_engine(host: str) -> None:
    adapter = _factory(
        "news_search",
        base_url=f"http://{host}:8888",
        engines=("duckduckgo news",),
        allowed_hosts=(host,),
    )

    assert isinstance(adapter, WebSearchAdapter)
    assert adapter.source_key == "news_search"
    seen: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(200, json={"results": [], "unresponsive_engines": []})

    adapter._transport = httpx.MockTransport(respond)
    page = adapter.fetch_page(_news_search_request())
    assert page.request_count == 1
    assert seen[0].url.host == host
    assert seen[0].url.port == 8888
    assert seen[0].url.path == "/search"
    assert seen[0].url.params["engines"] == "duckduckgo news"
    with pytest.raises(ValueError, match=r"allowlisted|SearXNG"):
        _factory(
            "news_search",
            base_url="http://searxng:8080",
            engines=("duckduckgo news",),
            allowed_hosts=("searxng",),
        )
    with pytest.raises(ValueError, match="duckduckgo news"):
        _factory(
            "news_search",
            base_url="http://127.0.0.1:8888",
            engines=("duckduckgo news", "bing news"),
            allowed_hosts=("127.0.0.1",),
        )


@pytest.mark.parametrize(
    ("base_url", "allowed_hosts"),
    [
        ("http://host.docker.internal:8889", ("host.docker.internal",)),
        ("https://host.docker.internal:8888", ("host.docker.internal",)),
        ("http://host.docker.internal:8888/search", ("host.docker.internal",)),
        ("http://host.docker.internal:8888/?q=AI", ("host.docker.internal",)),
        ("http://user@host.docker.internal:8888", ("host.docker.internal",)),
        ("http://host.docker.internal:8888", ("127.0.0.1",)),
        ("http://host.docker.internal:8888", ("127.0.0.1", "host.docker.internal")),
    ],
)
def test_web_search_factory_rejects_changes_to_fixed_endpoint(
    base_url: str, allowed_hosts: tuple[str, ...]
) -> None:
    with pytest.raises(ValueError):
        _factory(
            "news_search",
            base_url=base_url,
            engines=("duckduckgo news",),
            allowed_hosts=allowed_hosts,
        )


def _news_search_request(*, page_token: str | None = None, page_size: int = 2) -> SearchRequest:
    return SearchRequest(
        source_key="news_search",
        query="AI",
        page_size=page_size,
        page_token=page_token,
        starts_at=datetime(2026, 9, 25, tzinfo=UTC),
        ends_at=datetime(2026, 9, 26, tzinfo=UTC),
    )


def test_web_search_records_engine_and_normalizes_only_known_tracking_parameters() -> None:
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(
            200,
            json={
                "results": [
                    {
                        "engine": "duckduckgo news",
                        "url": "https://EXAMPLE.com:443/article?id=42&utm_source=mail&x=1#section",
                        "title": "AI story",
                        "content": "<b>Summary</b>",
                    },
                    {
                        "engine": "duckduckgo news",
                        "url": "https://example.com/article?id=42&x=1&utm_source=other",
                        "title": "AI story",
                        "publishedDate": None,
                    },
                ],
                "unresponsive_engines": [],
            },
        )

    adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(handler),
    )
    page = adapter.fetch_page(_news_search_request(page_size=3))
    assert page.state is SourcePageState.COMPLETE
    assert page.request_count == 1
    assert len(page.items) == 1
    item = page.items[0]
    assert isinstance(item, SourcePost)
    assert item.identity_basis == "url_fallback"
    assert item.canonical_url == "https://example.com/article?id=42&x=1"
    assert item.text == "Summary" and item.text_scope == "truncated"
    assert item.published_at is None
    assert page.source_engine == "duckduckgo news"
    assert page.source_page_number == 1
    assert seen[0].url.path == "/search"
    assert seen[0].url.params["q"] == "AI"
    assert seen[0].url.params["pageno"] == "1"
    assert seen[0].url.params["engines"] == "duckduckgo news"


@pytest.mark.parametrize(
    "payload",
    [
        {"results": [], "unresponsive_engines": [["duckduckgo news", "Timeout"]]},
        {"results": [], "error": "engine unavailable"},
    ],
)
def test_web_search_engine_failure_is_not_a_legitimate_empty_page(
    payload: dict[str, object],
) -> None:
    adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(lambda _request: httpx.Response(200, json=payload)),
    )
    page = adapter.fetch_page(_news_search_request())
    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.UPSTREAM_ERROR
    assert page.request_count == 1
    assert page.source_page_number == 1
    if "unresponsive_engines" in payload:
        assert page.source_unresponsive_engines == ("duckduckgo news: Timeout",)
    else:
        assert page.source_search_error == "engine unavailable"


def test_web_search_partial_engine_failure_keeps_results_and_failure() -> None:
    adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(
                200,
                json={
                    "results": [
                        {"engine": "duckduckgo news", "url": "https://example.com/a", "title": "AI"}
                    ],
                    "unresponsive_engines": [["duckduckgo news", "Timeout"]],
                },
            )
        ),
    )
    page = adapter.fetch_page(_news_search_request())
    assert page.state is SourcePageState.PARTIAL
    assert page.stop_reason is SourceStopReason.UPSTREAM_ERROR
    assert len(page.items) == 1 and page.request_count == 1
    assert page.source_unresponsive_engines == ("duckduckgo news: Timeout",)


def test_web_search_timeout_and_redirect_report_failures() -> None:
    timeout_adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: (_ for _ in ()).throw(httpx.ReadTimeout("slow engine"))
        ),
    )
    timeout = timeout_adapter.fetch_page(_news_search_request())
    assert timeout.state is SourcePageState.STOPPED
    assert timeout.stop_reason is SourceStopReason.UPSTREAM_ERROR
    assert timeout.request_count == 1
    assert timeout.source_search_error == "timeout"
    assert timeout.source_page_number == 1

    redirect_adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(302, headers={"location": "https://example.com/"})
        ),
    )
    redirect = redirect_adapter.fetch_page(_news_search_request())
    assert redirect.state is SourcePageState.STOPPED
    assert redirect.stop_reason is SourceStopReason.ACCESS_DENIED
    assert redirect.request_count == 1


def test_web_search_valid_empty_and_malformed_json_are_distinct() -> None:
    responses = iter(
        [
            httpx.Response(200, json={"results": [], "unresponsive_engines": []}),
            httpx.Response(200, text="not JSON"),
        ]
    )

    def handler(_request: httpx.Request) -> httpx.Response:
        return next(responses)

    empty_adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(handler),
    )
    broken_adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(handler),
    )
    empty = empty_adapter.fetch_page(_news_search_request())
    broken = broken_adapter.fetch_page(_news_search_request())
    assert (
        empty.state is SourcePageState.EMPTY and empty.stop_reason is SourceStopReason.SOURCE_EMPTY
    )
    assert (
        broken.state is SourcePageState.STOPPED
        and broken.stop_reason is SourceStopReason.PROTOCOL_ERROR
    )


def test_web_search_repeated_second_page_stops_without_recommitting_items() -> None:
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(
            200,
            json={
                "results": [
                    {"engine": "duckduckgo news", "url": "https://example.com/a", "title": "AI a"},
                    {"engine": "duckduckgo news", "url": "https://example.com/b", "title": "AI b"},
                ],
                "unresponsive_engines": [],
            },
        )

    adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(handler),
    )
    first = adapter.fetch_page(_news_search_request())
    second = adapter.fetch_page(_news_search_request(page_token=first.next_page_token))
    assert first.state is SourcePageState.MORE
    assert second.state is SourcePageState.STOPPED
    assert second.stop_reason is SourceStopReason.CURSOR_LOOP
    assert second.items == () and second.request_count == 1
    assert [request.url.params["pageno"] for request in seen] == ["1", "2"]


def test_web_search_page_overflow_marks_partial_without_silent_drop() -> None:
    adapter = WebSearchAdapter(
        source_key="news_search",
        base_url="http://127.0.0.1:8888",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _attempt: True,
        max_requests=4,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(
                200,
                json={
                    "results": [
                        {
                            "engine": "duckduckgo news",
                            "url": f"https://example.com/{index}",
                            "title": "AI",
                        }
                        for index in range(3)
                    ]
                },
            )
        ),
    )
    page = adapter.fetch_page(_news_search_request(page_size=2))
    assert page.state is SourcePageState.PARTIAL
    assert page.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert len(page.items) == 2 and page.next_page_token is None
    assert page.request_count == 1


def test_unknown_search_source_is_explicitly_rejected() -> None:
    with pytest.raises(UnsupportedSearchSourceError):
        build_search_adapter_factory(
            "unknown_source",
            SourceConnectionConfig(allowed_hosts=("example.com",)),
        )


def test_worker_registers_keyword_search_handler() -> None:
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")

    handlers = _registered_job_handlers(sessionmaker(), settings)

    assert "keyword.search" in handlers


def test_mediacrawler_live_search_without_per_request_guard_is_rejected(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        discovery_execution, "get_settings", lambda: SimpleNamespace(mediacrawler_enabled=True)
    )
    with pytest.raises(
        SearchRequestGuardUnavailableError, match="mediacrawler_request_guard_unavailable"
    ):
        build_search_adapter_factory(
            "bilibili",
            SourceConnectionConfig(
                base_url="https://www.bilibili.com",
                allowed_hosts=("www.bilibili.com",),
            ),
            owner_id=uuid4(),
        )


def test_bilibili_chrome_keeps_its_per_request_guard_when_mediacrawler_is_blocked(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    owner = uuid4()
    monkeypatch.setattr(
        discovery_execution,
        "get_settings",
        lambda: SimpleNamespace(
            bilibili_chrome_owner_id=owner,
            bilibili_chrome_identity_env="CONTROLLED_IDENTITY",
        ),
    )
    factory = build_search_adapter_factory(
        "bilibili",
        SourceConnectionConfig(
            base_url="https://api.bilibili.com",
            allowed_hosts=("api.bilibili.com",),
        ),
        owner_id=owner,
    )

    def before(_attempt: int) -> bool:
        return False

    def cancelled() -> bool:
        return True

    adapter = factory(before, cancelled, 1, 5)
    assert adapter._before_request is before
    assert adapter._cancelled is cancelled
