from __future__ import annotations

from dataclasses import dataclass

from sources.contracts import SourceCapability


@dataclass(frozen=True, slots=True)
class SourceCatalogEntry:
    source_key: str
    display_name: str
    rollout_role: str
    product_restricted: bool
    restricted_next_action: str


SOURCE_CATALOG = (
    SourceCatalogEntry(
        source_key="x",
        display_name="X",
        rollout_role="required",
        product_restricted=True,
        restricted_next_action="当前零采购约束下不启用付费 X API。请等待范围决策。",
    ),
    SourceCatalogEntry(
        source_key="douyin",
        display_name="抖音",
        rollout_role="candidate",
        product_restricted=False,
        restricted_next_action="确认开放平台权限、授权主体和当前准入政策后再验证。",
    ),
)

CAPABILITY_LABELS = {
    SourceCapability.SEARCH: "关键词检索",
    SourceCapability.AUTHOR_POSTS: "作者作品",
    SourceCapability.COMMENTS: "评论",
    SourceCapability.REPLIES: "回复",
}
