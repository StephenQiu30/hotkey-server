from __future__ import annotations

import json
from datetime import UTC, datetime

import httpx
import pytest

from sources.adapters.hackernews import HackerNewsAdapter
from sources.adapters.rss import RssSourceAdapter
from sources.adapters.web_search import WebSearchAdapter
from sources.contracts import (
    CommentsRequest,
    SearchRequest,
    SourceComment,
    SourcePageState,
    SourcePost,
    SourceSort,
    SourceStopReason,
)

_RSS = """<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>t</title>
<item><title>小米 SU7 &lt;b&gt;交付&lt;/b&gt;</title><link>https://news.example.com/a</link>
<guid>https://news.example.com/a</guid><pubDate>Fri, 25 Sep 2026 08:00:00 GMT</pubDate>
<description>&lt;p&gt;本周交付 1 万台&lt;/p&gt;</description><author>记者甲</author></item>
<item><title>重复</title><link>https://news.example.com/a</link>
<guid>https://news.example.com/a</guid></item>
<item><title>无日期</title><link>https://news.example.com/b</link></item>
</channel></rss>"""


def _allow(attempt: int) -> bool:
    return attempt <= 10


def _search(source_key: str, **changes: object) -> SearchRequest:
    values: dict[str, object] = {"source_key": source_key, "query": "小米 SU7", "page_size": 20}
    values.update(changes)
    return SearchRequest.model_validate(values)


def test_rss_search_encodes_query_and_maps_entries() -> None:
    seen: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(str(request.url))
        return httpx.Response(200, content=_RSS.encode())

    adapter = RssSourceAdapter(
        source_key="google_news",
        feed_url_template="https://news.google.com/rss/search?q={query}&hl=zh-CN",
        allowed_hosts=frozenset({"news.google.com"}),
        before_request=_allow,
        transport=httpx.MockTransport(handler),
    )
    page = adapter.fetch_page(_search("google_news"))

    assert seen == ["https://news.google.com/rss/search?q=%E5%B0%8F%E7%B1%B3%20SU7&hl=zh-CN"]
    assert page.state is SourcePageState.COMPLETE
    assert page.request_count == 1
    first, second = page.items
    assert isinstance(first, SourcePost)
    assert first.title == "小米 SU7 交付"
    assert first.text == "本周交付 1 万台"
    assert first.author_name == "记者甲"
    assert first.published_at == datetime(2026, 9, 25, 8, tzinfo=UTC)
    assert first.canonical_url == "https://news.example.com/a"
    assert second.external_id == "https://news.example.com/b"
    assert second.published_at is None
    assert adapter.fetch_page(_search("google_news")).request_count == 0


def test_rss_maps_http_failures_and_budget_to_stop_reasons() -> None:
    def rate_limited(request: httpx.Request) -> httpx.Response:
        return httpx.Response(429, headers={"retry-after": "30"})

    adapter = RssSourceAdapter(
        source_key="rsshub",
        feed_url_template="http://127.0.0.1:1200/zhihu/hot",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=_allow,
        transport=httpx.MockTransport(rate_limited),
    )
    page = adapter.fetch_page(_search("rsshub"))
    assert page.stop_reason is SourceStopReason.RATE_LIMITED
    assert page.retry_at is not None

    denied = RssSourceAdapter(
        source_key="rsshub",
        feed_url_template="http://127.0.0.1:1200/zhihu/hot",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda attempt: False,
        transport=httpx.MockTransport(lambda request: httpx.Response(200, content=b"")),
    )
    assert denied.fetch_page(_search("rsshub")).stop_reason is SourceStopReason.BUDGET_EXHAUSTED

    broken = RssSourceAdapter(
        source_key="rsshub",
        feed_url_template="http://127.0.0.1:1200/zhihu/hot",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=_allow,
        transport=httpx.MockTransport(lambda request: httpx.Response(200, content=b"<html")),
    )
    assert broken.fetch_page(_search("rsshub")).stop_reason is SourceStopReason.PROTOCOL_ERROR


def _hn_transport(calls: list[httpx.Request]) -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        if request.url.path.endswith("/search_by_date"):
            page = int(request.url.params["page"])
            hits = [
                {
                    "objectID": str(100 + page),
                    "title": f"Story {page}",
                    "url": "https://example.com/s",
                    "author": "pg",
                    "points": 42,
                    "num_comments": 3,
                    "created_at_i": 1790323200,
                }
            ]
            return httpx.Response(200, json={"hits": hits, "nbPages": 2})
        tree = {
            "id": 100,
            "children": [
                {
                    "id": 201,
                    "type": "comment",
                    "author": "a",
                    "text": "<p>First</p>",
                    "created_at_i": 1790323300,
                    "children": [
                        {
                            "id": 202,
                            "type": "comment",
                            "author": "b",
                            "text": "Reply",
                            "created_at_i": 1790323400,
                            "children": [],
                        }
                    ],
                },
                {"id": 203, "type": "comment", "author": None, "text": None, "children": []},
            ],
        }
        return httpx.Response(200, json=tree)

    return httpx.MockTransport(handler)


def test_hackernews_search_pages_and_window_filters() -> None:
    calls: list[httpx.Request] = []
    adapter = HackerNewsAdapter(
        allowed_hosts=frozenset({"hn.algolia.com"}),
        before_request=_allow,
        transport=_hn_transport(calls),
    )
    request = _search(
        "hackernews",
        page_size=1,
        sort=SourceSort.LATEST,
        starts_at=datetime(2026, 9, 25, tzinfo=UTC),
        ends_at=datetime(2026, 9, 26, tzinfo=UTC),
    )
    first = adapter.fetch_page(request)
    second = adapter.fetch_page(request.model_copy(update={"page_token": first.next_page_token}))

    assert first.state is SourcePageState.MORE and first.next_page_token == "1"
    assert second.state is SourcePageState.COMPLETE
    assert calls[0].url.params["numericFilters"] == (
        "created_at_i>=1790294400,created_at_i<1790380800"
    )
    post = first.items[0]
    assert isinstance(post, SourcePost)
    assert (post.external_id, post.title, post.like_count, post.comment_count) == (
        "100",
        "Story 0",
        42,
        3,
    )
    assert post.canonical_url == "https://news.ycombinator.com/item?id=100"


def test_hackernews_comments_keep_reply_parents_and_skip_deleted() -> None:
    calls: list[httpx.Request] = []
    adapter = HackerNewsAdapter(
        allowed_hosts=frozenset({"hn.algolia.com"}),
        before_request=_allow,
        transport=_hn_transport(calls),
    )
    request = CommentsRequest(source_key="hackernews", post_external_id="100", page_size=1)
    first = adapter.fetch_page(request)
    second = adapter.fetch_page(request.model_copy(update={"page_token": first.next_page_token}))

    assert len(calls) == 1
    comments = [*first.items, *second.items]
    assert all(isinstance(item, SourceComment) for item in comments)
    assert [
        (c.external_id, c.post_external_id, c.parent_comment_external_id, c.text)
        for c in comments
        if isinstance(c, SourceComment)
    ] == [("201", "100", None, "First"), ("202", "100", "201", "Reply")]
    assert second.state is SourcePageState.COMPLETE


def test_web_search_uses_searxng_json_and_dedupes_urls() -> None:
    calls: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        results = [
            {
                "url": "https://news.qq.com/a",
                "title": "SU7 销量",
                "content": "<b>九月</b>销量",
                "publishedDate": "2026-09-25T06:00:00",
            },
            {"url": "https://news.qq.com/a", "title": "dup"},
            {"url": "javascript:alert(1)", "title": "bad"},
            {
                "url": "https://sina.cn/b",
                "title": "SU7 评测",
                "publishedDate": "2026-09-25T07:00:00+08:00",
            },
        ]
        return httpx.Response(200, content=json.dumps({"results": results}).encode())

    adapter = WebSearchAdapter(
        base_url="http://127.0.0.1:8888/",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=_allow,
        transport=httpx.MockTransport(handler),
    )
    page = adapter.fetch_page(_search("web"))

    assert calls[0].url.params["format"] == "json"
    assert calls[0].url.params["engines"] == "duckduckgo news"
    assert "categories" not in calls[0].url.params
    assert page.next_page_token == "2"
    naive, aware = page.items
    assert isinstance(naive, SourcePost) and isinstance(aware, SourcePost)
    assert naive.published_at == datetime(2026, 9, 25, 6, tzinfo=UTC)
    assert aware.published_at == datetime(2026, 9, 24, 23, tzinfo=UTC)
    assert aware.canonical_url == "https://sina.cn/b"
    assert naive.text == "九月 销量"


def test_redirect_to_host_outside_allowlist_is_denied() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        if request.url.host != "feeds.example.com":
            pytest.fail("redirect target must be rejected before a request is sent")
        return httpx.Response(302, headers={"location": "https://outside.example/rss"})

    adapter = RssSourceAdapter(
        source_key="rss",
        feed_url_template="https://feeds.example.com/rss",
        allowed_hosts=frozenset({"feeds.example.com"}),
        before_request=_allow,
        max_requests=6,
        transport=httpx.MockTransport(handler),
    )

    page = adapter.fetch_page(_search("rss"))

    assert page.stop_reason is SourceStopReason.ACCESS_DENIED
    assert page.request_count == 1


def test_redirect_to_private_address_is_denied_even_when_allowlisted() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        if request.url.host != "feeds.example.com":
            pytest.fail("private redirect target must be rejected before a request is sent")
        return httpx.Response(302, headers={"location": "http://169.254.169.254/latest"})

    adapter = RssSourceAdapter(
        source_key="rss",
        feed_url_template="https://feeds.example.com/rss",
        allowed_hosts=frozenset({"feeds.example.com", "169.254.169.254"}),
        before_request=_allow,
        max_requests=6,
        transport=httpx.MockTransport(handler),
    )

    page = adapter.fetch_page(_search("rss"))

    assert page.stop_reason is SourceStopReason.ACCESS_DENIED
    assert page.request_count == 1


def test_allowlisted_redirects_are_followed_for_at_most_five_hops() -> None:
    paths: list[str] = []

    def succeeds_after_redirect(request: httpx.Request) -> httpx.Response:
        paths.append(request.url.path)
        if request.url.path == "/rss":
            return httpx.Response(302, headers={"location": "/final"})
        return httpx.Response(200, content=_RSS.encode())

    adapter = RssSourceAdapter(
        source_key="rss",
        feed_url_template="https://feeds.example.com/rss",
        allowed_hosts=frozenset({"feeds.example.com"}),
        before_request=_allow,
        max_requests=6,
        transport=httpx.MockTransport(succeeds_after_redirect),
    )

    page = adapter.fetch_page(_search("rss"))

    assert paths == ["/rss", "/final"]
    assert page.state is SourcePageState.COMPLETE
    assert page.request_count == 2

    redirect_requests = 0

    def never_finishes(request: httpx.Request) -> httpx.Response:
        nonlocal redirect_requests
        redirect_requests += 1
        return httpx.Response(302, headers={"location": f"/redirect/{redirect_requests}"})

    limited = RssSourceAdapter(
        source_key="rss",
        feed_url_template="https://feeds.example.com/rss",
        allowed_hosts=frozenset({"feeds.example.com"}),
        before_request=_allow,
        max_requests=7,
        transport=httpx.MockTransport(never_finishes),
    )

    stopped = limited.fetch_page(_search("rss"))

    assert stopped.stop_reason is SourceStopReason.PROTOCOL_ERROR
    assert stopped.request_count == 6
    assert redirect_requests == 6


def test_configured_endpoint_hosts_must_be_allowlisted() -> None:
    with pytest.raises(ValueError, match="feed_url_template host must be allowlisted"):
        RssSourceAdapter(
            source_key="rss",
            feed_url_template="https://feeds.example.com/rss",
            allowed_hosts=frozenset({"other.example"}),
            before_request=_allow,
        )

    with pytest.raises(ValueError, match="base_url host must be allowlisted"):
        WebSearchAdapter(
            base_url="http://127.0.0.1:8888",
            allowed_hosts=frozenset({"localhost"}),
            before_request=_allow,
        )
