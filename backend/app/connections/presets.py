from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from types import MappingProxyType

from sources.contracts import SourceCapability


@dataclass(frozen=True, slots=True)
class SourceCapabilityPreset:
    capability: SourceCapability
    processing_purpose: str
    field_purposes: Mapping[str, str]


@dataclass(frozen=True, slots=True)
class SourceBudgetPreset:
    budget_key: str
    metric: str
    scope_kind: str
    scope_reference: str
    limit_units: int
    window_seconds: int
    window_anchor_at: datetime


@dataclass(frozen=True, slots=True)
class SourcePreset:
    source_key: str
    config: Mapping[str, object]
    capabilities: tuple[SourceCapabilityPreset, ...]
    retention_days: int
    component_name: str
    component_version: str
    component_license: str
    component_cost_class: str
    component_terms_reference: str
    access_terms_reference: str
    reviewed_at: datetime
    budget: SourceBudgetPreset


_POST_FIELD_PURPOSES = MappingProxyType(
    {
        "object_type": "区分帖子与评论载荷",
        "external_id": "稳定识别来源帖子",
        "canonical_url": "回溯原始帖子",
        "author_external_id": "稳定识别公开作者",
        "author_name": "展示公开作者昵称",
        "published_at": "按来源发布时间排序与统计",
        "like_count": "记录点赞互动观察",
        "comment_count": "记录评论互动观察",
        "repost_count": "记录转发互动观察",
        "view_count": "记录浏览互动观察",
        "play_count": "记录播放互动观察",
        "danmaku_count": "记录弹幕互动观察",
        "text_scope": "标记正文完整性",
        "text_origin": "标记正文来源",
        "title": "保存帖子标题",
        "body": "保存帖子正文",
        "truncation_reason": "解释正文截断原因",
        "quote_target_external_id": "关联引用的来源帖子",
        "repost_target_external_id": "关联转发的来源帖子",
    }
)

_COMMENT_FIELD_PURPOSES = MappingProxyType(
    {
        "object_type": "区分帖子与评论载荷",
        "external_id": "稳定识别来源评论",
        "post_external_id": "关联评论所属帖子",
        "parent_comment_external_id": "恢复楼中楼父子关系",
        "canonical_url": "回溯原始评论",
        "author_external_id": "稳定识别公开作者",
        "author_name": "展示公开作者昵称",
        "published_at": "按来源发布时间排序与统计",
        "like_count": "记录评论点赞观察",
        "text_scope": "标记评论正文完整性",
        "text_origin": "标记评论正文来源",
        "body": "保存评论正文",
        "truncation_reason": "解释评论正文截断原因",
    }
)

_RSS_POST_FIELD_PURPOSES = MappingProxyType(
    {
        "object_type": "标记载荷为帖子",
        "external_id": "稳定识别订阅条目",
        "canonical_url": "回溯订阅条目指向的原始页面",
        "author_name": "保存订阅条目提供的公开作者名称",
        "published_at": "保存订阅条目提供的发布时间",
        "like_count": "记录 RSS 不提供点赞指标的缺失值",
        "comment_count": "记录 RSS 不提供评论指标的缺失值",
        "repost_count": "记录 RSS 不提供转发指标的缺失值",
        "view_count": "记录 RSS 不提供浏览指标的缺失值",
        "play_count": "记录 RSS 不提供播放指标的缺失值",
        "danmaku_count": "记录 RSS 不提供弹幕指标的缺失值",
        "text_scope": "标记订阅摘要的正文完整性",
        "text_origin": "标记正文来自订阅源",
        "title": "保存订阅条目标题",
        "body": "保存订阅条目摘要",
        "truncation_reason": "说明订阅摘要受来源限制",
    }
)

_WEB_SEARCH_POST_FIELD_PURPOSES = MappingProxyType(
    {
        "object_type": "标记载荷为帖子",
        "external_id": "按结果链接稳定识别新闻结果",
        "canonical_url": "回溯新闻搜索结果指向的原始页面",
        "published_at": "保存搜索引擎提供的发布时间",
        "like_count": "记录 SearXNG 不提供点赞指标的缺失值",
        "comment_count": "记录 SearXNG 不提供评论指标的缺失值",
        "repost_count": "记录 SearXNG 不提供转发指标的缺失值",
        "view_count": "记录 SearXNG 不提供浏览指标的缺失值",
        "play_count": "记录 SearXNG 不提供播放指标的缺失值",
        "danmaku_count": "记录 SearXNG 不提供弹幕指标的缺失值",
        "text_scope": "标记搜索摘要的正文完整性",
        "text_origin": "标记正文来自搜索结果摘要",
        "title": "保存新闻搜索结果标题",
        "body": "保存新闻搜索结果摘要",
        "truncation_reason": "说明搜索摘要受来源限制",
    }
)

_A_TIER_REVIEWED_AT = datetime(2026, 9, 25, tzinfo=UTC)

HACKERNEWS_PRESET = SourcePreset(
    source_key="hackernews",
    config=MappingProxyType(
        {
            "base_url": "https://hn.algolia.com/api/v1",
            "allowed_hosts": ("hn.algolia.com",),
        }
    ),
    capabilities=(
        SourceCapabilityPreset(
            capability=SourceCapability.SEARCH,
            processing_purpose="发现与已配置主题相关的公开 Hacker News 帖子",
            field_purposes=_POST_FIELD_PURPOSES,
        ),
        SourceCapabilityPreset(
            capability=SourceCapability.COMMENTS,
            processing_purpose="采集已入库 Hacker News 帖子的公开评论与回复关系",
            field_purposes=_COMMENT_FIELD_PURPOSES,
        ),
    ),
    retention_days=90,
    component_name="collector.hackernews",
    component_version="hn-algolia-v1",
    component_license="MIT",
    component_cost_class="zero_price",
    component_terms_reference="https://hn.algolia.com/api",
    access_terms_reference="https://hn.algolia.com/api",
    reviewed_at=_A_TIER_REVIEWED_AT,
    budget=SourceBudgetPreset(
        budget_key="source.hackernews.network.daily",
        metric="network_request",
        scope_kind="source",
        scope_reference="hackernews",
        limit_units=1_000,
        window_seconds=86_400,
        window_anchor_at=datetime(2026, 1, 1, tzinfo=UTC),
    ),
)


GOOGLE_NEWS_PRESET = SourcePreset(
    source_key="google_news",
    config=MappingProxyType(
        {
            "feed_url_template": (
                "https://news.google.com/rss/search?q={query}&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"
            ),
            "allowed_hosts": ("news.google.com",),
        }
    ),
    capabilities=(
        SourceCapabilityPreset(
            capability=SourceCapability.SEARCH,
            processing_purpose="通过 Google News 搜索 RSS 发现与主题相关的公开新闻条目",
            field_purposes=_RSS_POST_FIELD_PURPOSES,
        ),
    ),
    retention_days=90,
    component_name="collector.google_news",
    component_version="feedparser-6",
    component_license="BSD-2-Clause",
    component_cost_class="zero_price",
    component_terms_reference="https://news.google.com/rss",
    access_terms_reference="https://news.google.com/rss",
    reviewed_at=_A_TIER_REVIEWED_AT,
    budget=SourceBudgetPreset(
        budget_key="source.google_news.network.daily",
        metric="network_request",
        scope_kind="source",
        scope_reference="google_news",
        limit_units=1_000,
        window_seconds=86_400,
        window_anchor_at=datetime(2026, 1, 1, tzinfo=UTC),
    ),
)


NEWS_SEARCH_PRESET = SourcePreset(
    source_key="news_search",
    config=MappingProxyType(
        {
            "base_url": "http://127.0.0.1:8888",
            "engines": ("duckduckgo news",),
            "allowed_hosts": ("127.0.0.1",),
        }
    ),
    capabilities=(
        SourceCapabilityPreset(
            capability=SourceCapability.SEARCH,
            processing_purpose="通过本地 SearXNG 新闻引擎发现与主题相关的公开新闻页面",
            field_purposes=_WEB_SEARCH_POST_FIELD_PURPOSES,
        ),
    ),
    retention_days=90,
    component_name="collector.news_search",
    component_version="searxng-json-news",
    component_license="AGPL-3.0-or-later",
    component_cost_class="zero_price",
    component_terms_reference="https://docs.searxng.org/dev/search_api.html",
    access_terms_reference="https://docs.searxng.org/dev/search_api.html",
    reviewed_at=_A_TIER_REVIEWED_AT,
    budget=SourceBudgetPreset(
        budget_key="source.news_search.network.daily",
        metric="network_request",
        scope_kind="source",
        scope_reference="news_search",
        limit_units=1_000,
        window_seconds=86_400,
        window_anchor_at=datetime(2026, 1, 1, tzinfo=UTC),
    ),
)


RSS_36KR_PRESET = SourcePreset(
    source_key="rss_36kr",
    config=MappingProxyType(
        {
            # 36kr.com/feed now answers with an HTML challenge page; read it via local RSSHub.
            "feed_url_template": "http://127.0.0.1:1200/36kr/newsflashes",
            "allowed_hosts": ("127.0.0.1",),
        }
    ),
    capabilities=(
        SourceCapabilityPreset(
            capability=SourceCapability.SEARCH,
            processing_purpose="经本地 RSSHub 读取 36Kr 快讯并筛选与主题相关的公开行业资讯",
            field_purposes=_RSS_POST_FIELD_PURPOSES,
        ),
    ),
    retention_days=90,
    component_name="collector.rss_36kr",
    component_version="rsshub-36kr-newsflashes",
    component_license="AGPL-3.0",
    component_cost_class="zero_price",
    component_terms_reference="https://docs.rsshub.app/routes/new-media#36kr",
    access_terms_reference="https://36kr.com/newsflashes",
    reviewed_at=_A_TIER_REVIEWED_AT,
    budget=SourceBudgetPreset(
        budget_key="source.rss_36kr.network.daily",
        metric="network_request",
        scope_kind="source",
        scope_reference="rss_36kr",
        limit_units=1_000,
        window_seconds=86_400,
        window_anchor_at=datetime(2026, 1, 1, tzinfo=UTC),
    ),
)


SOURCE_PRESETS: Mapping[str, SourcePreset] = MappingProxyType(
    {
        preset.source_key: preset
        for preset in (
            HACKERNEWS_PRESET,
            GOOGLE_NEWS_PRESET,
            NEWS_SEARCH_PRESET,
            RSS_36KR_PRESET,
        )
    }
)
