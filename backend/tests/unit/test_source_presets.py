from __future__ import annotations

import re
from datetime import UTC, datetime
from urllib.parse import urlsplit

import httpx
import pytest
from pydantic import ValidationError
from sqlalchemy.dialects.postgresql.psycopg import PGDialect_psycopg
from typer.testing import CliRunner

from cli.commands import app
from connections.catalog import SOURCE_CATALOG
from connections.models import SourceConnectionVersion
from connections.presets import (
    BILIBILI_PRESET,
    GOOGLE_NEWS_PRESET,
    HACKERNEWS_PRESET,
    NEWS_SEARCH_PRESET,
    RSS_36KR_PRESET,
    SOURCE_PRESETS,
    SourcePreset,
)
from connections.schemas import SourceConnectionConfig, SourceExecutionPolicy
from content.discovery import KeywordDiscoveryPageCommitService
from content.schemas import PersistContentPostInput
from content.services import _ALLOWED_FIELDS
from sources.adapters.hackernews import HackerNewsAdapter
from sources.adapters.rss import RssSourceAdapter
from sources.adapters.web_search import WebSearchAdapter
from sources.contracts import SearchRequest, SourceCapability, SourcePost

_RSS_WITH_AUTHOR = b"""<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>HotKey</title><item>
<title>AI industry update</title><link>https://example.com/posts/1</link>
<guid>post-1</guid><pubDate>Fri, 25 Sep 2026 08:00:00 GMT</pubDate>
<description>Daily update</description><author>Reporter</author>
</item></channel></rss>"""


def test_optional_execution_policy_binds_python_none_as_sql_null() -> None:
    dialect = PGDialect_psycopg()
    column = SourceConnectionVersion.__table__.c.execution_policy
    processor = column.type.dialect_impl(dialect).bind_processor(dialect)
    assert column.nullable
    assert not SourceConnectionVersion.__table__.c.config.nullable
    assert processor is not None
    assert processor(None) is None
    assert processor(BILIBILI_PRESET.execution_policy.model_dump(mode="json")) is not None


@pytest.mark.parametrize(
    "change",
    [
        {"max_queries": 0},
        {"max_items_per_query": -1},
        {"max_requests": 0},
        {"max_seconds": 241},
        {"hard_timeout_seconds": 0},
        {"max_concurrency": 0},
        {"surprise": "value"},
    ],
)
def test_execution_policy_rejects_invalid_limits_and_unknown_fields(
    change: dict[str, object],
) -> None:
    values = BILIBILI_PRESET.execution_policy.model_dump() | change
    with pytest.raises(ValidationError):
        SourceExecutionPolicy.model_validate(values)


def test_bilibili_execution_policy_is_bounded_and_quiet_in_shanghai() -> None:
    policy = BILIBILI_PRESET.execution_policy
    assert (
        policy.min_interval_seconds,
        policy.max_queries,
        policy.max_items_per_query,
        policy.max_requests,
        policy.max_seconds,
        policy.hard_timeout_seconds,
        policy.max_concurrency,
        policy.enabled,
    ) == (21_600, 3, 5, 26, 220, 240, 1, True)
    assert policy.quiet_windows[0].timezone == "Asia/Shanghai"
    assert policy.quiet_at(datetime(2026, 9, 25, 16, 0, tzinfo=UTC))
    assert policy.quiet_at(datetime(2026, 9, 25, 23, 59, tzinfo=UTC))
    assert not policy.quiet_at(datetime(2026, 9, 26, 0, 0, tzinfo=UTC))
    crossing = SourceExecutionPolicy.model_validate(
        policy.model_dump()
        | {"quiet_windows": [{"timezone": "Asia/Shanghai", "start": "22:00", "end": "02:00"}]}
    )
    assert crossing.quiet_at(datetime(2026, 9, 26, 15, 0, tzinfo=UTC))
    assert crossing.quiet_at(datetime(2026, 9, 25, 17, 0, tzinfo=UTC))
    assert not crossing.quiet_at(datetime(2026, 9, 25, 18, 0, tzinfo=UTC))


def test_keyword_and_hotlist_execution_policy_snapshots() -> None:
    for key in ("hackernews", "google_news", "news_search", "rss_36kr"):
        preset = SOURCE_PRESETS[key]
        assert preset.budget.limit_units == 1_000
        assert preset.budget.window_seconds == 86_400
        assert preset.execution_policy.max_items_per_query == 100
        assert preset.execution_policy.max_requests == 3
        assert preset.execution_policy.max_seconds == 90
    for preset in SOURCE_PRESETS.values():
        if preset.source_key.startswith("hotlist_"):
            assert preset.execution_policy.min_interval_seconds == 1_800
            assert preset.budget.limit_units == 500


def _search(source_key: str) -> SearchRequest:
    return SearchRequest(source_key=source_key, query="AI", page_size=20)


def _admission_payload_fields(post: SourcePost) -> set[str]:
    return set(KeywordDiscoveryPageCommitService._payload(post))


def _assert_config_is_allowlisted_and_host_is_covered(preset: SourcePreset) -> None:
    assert set(preset.config) <= set(SourceConnectionConfig.model_fields)
    config = SourceConnectionConfig.model_validate(dict(preset.config))
    endpoint = (
        config.feed_url
        or config.feed_url_template
        or (str(config.base_url) if config.base_url else None)
    )
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
        "hotlist_weibo",
        "hotlist_baidu",
        "hotlist_zhihu",
        "hotlist_bilibili",
        "hotlist_36kr",
        "hotlist_thepaper",
        "hackernews",
        "bilibili",
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


def test_bilibili_preset_is_separate_from_hotlist_and_caps_daily_requests() -> None:
    catalog = next(item for item in SOURCE_CATALOG if item.source_key == "bilibili")

    assert BILIBILI_PRESET.source_key != "hotlist_bilibili"
    assert BILIBILI_PRESET.component_name == "collector.bilibili"
    assert BILIBILI_PRESET.component_version == "mediacrawler-380b426-hotkey-safe"
    assert BILIBILI_PRESET.budget.limit_units == 60
    assert {item.capability for item in BILIBILI_PRESET.capabilities} == {
        SourceCapability.SEARCH,
        SourceCapability.COMMENTS,
    }
    assert set(catalog.capabilities) == {
        SourceCapability.SEARCH,
        SourceCapability.COMMENTS,
    }
    _assert_config_is_allowlisted_and_host_is_covered(BILIBILI_PRESET)


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
        "hotlist_weibo; capabilities: hotlist; allowed hosts: 127.0.0.1\n"
        "hotlist_baidu; capabilities: hotlist; allowed hosts: 127.0.0.1\n"
        "hotlist_zhihu; capabilities: hotlist; allowed hosts: 127.0.0.1\n"
        "hotlist_bilibili; capabilities: hotlist; allowed hosts: 127.0.0.1\n"
        "hotlist_36kr; capabilities: hotlist; allowed hosts: 127.0.0.1\n"
        "hotlist_thepaper; capabilities: hotlist; allowed hosts: 127.0.0.1\n"
        "hackernews; capabilities: search,comments; allowed hosts: hn.algolia.com\n"
        "bilibili; capabilities: search,comments; allowed hosts: "
        "api.bilibili.com,www.bilibili.com\n"
        "google_news; capabilities: search; allowed hosts: news.google.com\n"
        "news_search; capabilities: search; allowed hosts: 127.0.0.1\n"
        "rss_36kr; capabilities: search; allowed hosts: 127.0.0.1\n"
    )


@pytest.mark.parametrize("preset", list(SOURCE_PRESETS.values()))
def test_preset_component_identity_is_accepted_when_saving_content(preset: SourcePreset) -> None:
    """Saved posts carry the preset component name and version; both must pass the save schema."""
    fields = PersistContentPostInput.model_fields
    for field_name, value in (
        ("component_name", preset.component_name),
        ("component_version", preset.component_version),
    ):
        pattern = next(
            item.pattern for item in fields[field_name].metadata if hasattr(item, "pattern")
        )
        assert re.fullmatch(pattern, value), f"{preset.source_key} {field_name}={value!r}"


@pytest.mark.parametrize("adapter", [HackerNewsAdapter, RssSourceAdapter, WebSearchAdapter])
def test_adapter_version_is_accepted_when_saving_content(adapter: type) -> None:
    """Discovery saves each page's adapter_version as the post's component version."""
    field = PersistContentPostInput.model_fields["component_version"]
    pattern = next(item.pattern for item in field.metadata if hasattr(item, "pattern"))
    assert re.fullmatch(pattern, adapter.adapter_version), adapter.adapter_version
