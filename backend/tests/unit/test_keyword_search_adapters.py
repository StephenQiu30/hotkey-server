from __future__ import annotations

import pytest
from sqlalchemy.orm import sessionmaker

from connections.schemas import SourceConnectionConfig
from content.discovery_execution import (
    UnsupportedSearchSourceError,
    build_search_adapter_factory,
)
from core.config import Settings
from sources.adapters.hackernews import HackerNewsAdapter
from sources.adapters.rss import RssSourceAdapter
from sources.adapters.web_search import WebSearchAdapter
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


def test_rss_factory_uses_connection_feed_template() -> None:
    adapter = _factory(
        "google_news",
        feed_url_template="https://news.google.com/rss/search?q={query}",
        allowed_hosts=("news.google.com",),
    )

    assert isinstance(adapter, RssSourceAdapter)
    assert adapter.source_key == "google_news"
    assert adapter._template == "https://news.google.com/rss/search?q={query}"


def test_web_search_factory_uses_connection_base_url_and_engines() -> None:
    adapter = _factory(
        "news_search",
        base_url="http://searxng:8080",
        engines=("duckduckgo news", "bing news"),
        allowed_hosts=("searxng",),
    )

    assert isinstance(adapter, WebSearchAdapter)
    assert adapter.source_key == "news_search"
    assert adapter._search_url == "http://searxng:8080/search"
    assert adapter._engines == "duckduckgo news,bing news"


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
