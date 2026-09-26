from __future__ import annotations

from urllib.parse import urlsplit

import httpx
import pytest
from pydantic import ValidationError
from typer.testing import CliRunner

from cli.commands import app
from connections.catalog import SOURCE_CATALOG
from connections.presets import (
    GOOGLE_NEWS_PRESET,
    HACKERNEWS_PRESET,
    NEWS_SEARCH_PRESET,
    RSS_36KR_PRESET,
    SOURCE_PRESETS,
    SourcePreset,
)
from connections.schemas import SourceConnectionConfig
from content.discovery import KeywordDiscoveryPageCommitService
from content.services import _ALLOWED_FIELDS
from sources.adapters.rss import RssSourceAdapter
from sources.adapters.web_search import WebSearchAdapter
from sources.contracts import SearchRequest, SourceCapability, SourcePost

_RSS_WITH_AUTHOR = b"""<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>HotKey</title><item>
<title>AI industry update</title><link>https://example.com/posts/1</link>
<guid>post-1</guid><pubDate>Fri, 25 Sep 2026 08:00:00 GMT</pubDate>
<description>Daily update</description><author>Reporter</author>
</item></channel></rss>"""


def _search(source_key: str) -> SearchRequest:
    return SearchRequest(source_key=source_key, query="AI", page_size=20)


def _admission_payload_fields(post: SourcePost) -> set[str]:
    return set(KeywordDiscoveryPageCommitService._payload(post))


def _assert_config_is_allowlisted_and_host_is_covered(preset: SourcePreset) -> None:
    assert set(preset.config) <= set(SourceConnectionConfig.model_fields)
    config = SourceConnectionConfig.model_validate(dict(preset.config))
    endpoint = config.feed_url_template or (str(config.base_url) if config.base_url else None)
    assert endpoint is not None
    hostname = urlsplit(endpoint).hostname
    assert hostname is not None
    assert hostname in config.allowed_hosts


def _assert_search_fields_cover_post(preset: SourcePreset, post: SourcePost) -> None:
    capability = next(
        item for item in preset.capabilities if item.capability is SourceCapability.SEARCH
    )
    assert _admission_payload_fields(post) <= set(capability.field_purposes)
    assert set(capability.field_purposes) <= _ALLOWED_FIELDS


def test_hackernews_preset_covers_persisted_post_and_comment_fields() -> None:
    capabilities = {
        item.capability: set(item.field_purposes) for item in HACKERNEWS_PRESET.capabilities
    }

    assert set(capabilities) == {SourceCapability.SEARCH, SourceCapability.COMMENTS}
    assert capabilities[SourceCapability.SEARCH] >= {
        "external_id",
        "author_name",
        "title",
        "body",
        "view_count",
        "comment_count",
    }
    assert capabilities[SourceCapability.COMMENTS] >= {
        "external_id",
        "post_external_id",
        "parent_comment_external_id",
        "author_name",
        "body",
    }
    assert set().union(*capabilities.values()) <= _ALLOWED_FIELDS


def test_hackernews_preset_matches_catalog_and_config_allowlist() -> None:
    catalog = next(item for item in SOURCE_CATALOG if item.source_key == "hackernews")
    config = SourceConnectionConfig.model_validate(dict(HACKERNEWS_PRESET.config))

    assert SOURCE_PRESETS["hackernews"] is HACKERNEWS_PRESET
    assert set(SOURCE_PRESETS) == {
        "hackernews",
        "google_news",
        "news_search",
        "rss_36kr",
    }
    assert set(catalog.capabilities) == {
        SourceCapability.SEARCH,
        SourceCapability.COMMENTS,
    }
    assert config.allowed_hosts == ("hn.algolia.com",)
    assert str(config.base_url) == "https://hn.algolia.com/api/v1"


@pytest.mark.parametrize(
    ("preset", "expected_capabilities", "expected_endpoint", "expected_engines"),
    [
        (
            GOOGLE_NEWS_PRESET,
            {SourceCapability.SEARCH},
            "https://news.google.com/rss/search?q={query}&hl=zh-CN&gl=CN&ceid=CN:zh-Hans",
            (),
        ),
        (
            NEWS_SEARCH_PRESET,
            {SourceCapability.SEARCH},
            "http://127.0.0.1:8888",
            ("duckduckgo news",),
        ),
        (
            RSS_36KR_PRESET,
            {SourceCapability.SEARCH},
            "http://127.0.0.1:1200/36kr/newsflashes",
            (),
        ),
    ],
)
def test_a_tier_preset_matches_catalog_and_config_allowlist(
    preset: SourcePreset,
    expected_capabilities: set[SourceCapability],
    expected_endpoint: str,
    expected_engines: tuple[str, ...],
) -> None:
    catalog = next(item for item in SOURCE_CATALOG if item.source_key == preset.source_key)
    config = SourceConnectionConfig.model_validate(dict(preset.config))
    endpoint = config.feed_url_template or str(config.base_url).rstrip("/")

    _assert_config_is_allowlisted_and_host_is_covered(preset)
    assert {item.capability for item in preset.capabilities} == expected_capabilities
    assert set(catalog.capabilities) == expected_capabilities
    assert endpoint == expected_endpoint
    assert config.engines == expected_engines


@pytest.mark.parametrize("preset", list(SOURCE_PRESETS.values()))
def test_every_preset_uses_only_allowlisted_config_keys_and_covers_endpoint_host(
    preset: SourcePreset,
) -> None:
    _assert_config_is_allowlisted_and_host_is_covered(preset)


@pytest.mark.parametrize("preset", [GOOGLE_NEWS_PRESET, RSS_36KR_PRESET])
def test_rss_preset_fields_cover_adapter_payload_without_interaction_values(
    preset: SourcePreset,
) -> None:
    config = SourceConnectionConfig.model_validate(dict(preset.config))
    assert config.feed_url_template is not None
    adapter = RssSourceAdapter(
        source_key=preset.source_key,
        feed_url_template=config.feed_url_template,
        allowed_hosts=frozenset(config.allowed_hosts),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(200, content=_RSS_WITH_AUTHOR)
        ),
    )

    post = adapter.fetch_page(_search(preset.source_key)).items[0]

    assert isinstance(post, SourcePost)
    assert (
        post.like_count,
        post.comment_count,
        post.repost_count,
        post.view_count,
        post.play_count,
        post.danmaku_count,
    ) == (None, None, None, None, None, None)
    _assert_search_fields_cover_post(preset, post)


def test_news_search_preset_fields_cover_adapter_payload_without_interaction_values() -> None:
    config = SourceConnectionConfig.model_validate(dict(NEWS_SEARCH_PRESET.config))
    assert config.base_url is not None
    adapter = WebSearchAdapter(
        source_key=NEWS_SEARCH_PRESET.source_key,
        base_url=str(config.base_url),
        engines=",".join(config.engines),
        allowed_hosts=frozenset(config.allowed_hosts),
        before_request=lambda _attempt: True,
        transport=httpx.MockTransport(
            lambda _request: httpx.Response(
                200,
                json={
                    "results": [
                        {
                            "url": "https://example.com/posts/1",
                            "title": "AI industry update",
                            "content": "Daily update",
                            "publishedDate": "2026-09-25T08:00:00+00:00",
                        }
                    ]
                },
            )
        ),
    )

    post = adapter.fetch_page(_search(NEWS_SEARCH_PRESET.source_key)).items[0]

    assert isinstance(post, SourcePost)
    assert (
        post.like_count,
        post.comment_count,
        post.repost_count,
        post.view_count,
        post.play_count,
        post.danmaku_count,
    ) == (None, None, None, None, None, None)
    _assert_search_fields_cover_post(NEWS_SEARCH_PRESET, post)


def test_connection_config_rejects_non_allowlisted_keys() -> None:
    with pytest.raises(ValidationError):
        SourceConnectionConfig.model_validate(
            {
                "allowed_hosts": ["hn.algolia.com"],
                "api_token": "must-not-be-accepted",
            }
        )

    with pytest.raises(ValidationError):
        SourceConnectionConfig.model_validate(
            {
                "base_url": "https://user:password@hn.algolia.com/api/v1",
                "allowed_hosts": ["hn.algolia.com"],
            }
        )

    with pytest.raises(ValidationError):
        SourceConnectionConfig.model_validate(
            {
                "base_url": "http://10.0.0.1:8888",
                "allowed_hosts": ["10.0.0.1"],
            }
        )


def test_source_preset_cli_lists_built_in_presets() -> None:
    result = CliRunner().invoke(app, ["sources", "preset", "list"])

    assert result.exit_code == 0, result.output
    assert result.stdout == (
        "hackernews; capabilities: search,comments; allowed hosts: hn.algolia.com\n"
        "google_news; capabilities: search; allowed hosts: news.google.com\n"
        "news_search; capabilities: search; allowed hosts: 127.0.0.1\n"
        "rss_36kr; capabilities: search; allowed hosts: 127.0.0.1\n"
    )
