from datetime import date

from sources.schemas import (
    QueryPreview,
    SearchInput,
    SourceName,
    SourceOperationCapability,
    SourceView,
)

EVIDENCE = "docs/operations/evidence/007/EV-007-001-source-poc.json"
VERIFIED_AT = date(2026, 9, 15)


def capability(
    operation: str,
    support: str,
    access_mode: str,
    note: str,
) -> SourceOperationCapability:
    return SourceOperationCapability.model_validate(
        {
            "operation": operation,
            "support": support,
            "rights": "unknown",
            "access_mode": access_mode,
            "content_purchase_cost": 0,
            "verified_at": VERIFIED_AT,
            "evidence_ref": EVIDENCE,
            "note": note,
        }
    )


def operations(support: str, access_mode: str, note: str) -> list[SourceOperationCapability]:
    return [
        capability(operation, support, access_mode, note)
        for operation in ("search_posts", "fetch_post", "list_comments", "list_replies")
    ]


class SourceService:
    def catalog(self) -> list[SourceView]:
        rows: tuple[tuple[SourceName, list[str], list[SourceOperationCapability]], ...] = (
            (
                "x",
                ["discovery"],
                [
                    capability(
                        "search_posts",
                        "authorization_required",
                        "public_web",
                        "公开搜索只返回登录或挑战应用壳，未取得可解析帖子。",
                    ),
                    *[
                        capability(
                            operation,
                            "unknown",
                            "official_paid_api",
                            "零内容采购预算下未验证该操作。",
                        )
                        for operation in ("fetch_post", "list_comments", "list_replies")
                    ],
                ],
            ),
            (
                "bilibili",
                ["discovery", "comments"],
                operations(
                    "supported",
                    "public_web",
                    "无Cookie小样本可解析；长期保存与派生分析权限仍待核对。",
                ),
            ),
            (
                "weibo",
                ["comments"],
                [
                    capability(
                        "search_posts",
                        "authorization_required",
                        "authorized_session",
                        "公开搜索跳转平台登录。",
                    ),
                    *[
                        capability(
                            operation,
                            "unknown",
                            "authorized_session",
                            "未获得授权会话，未验证该操作结构。",
                        )
                        for operation in ("fetch_post", "list_comments", "list_replies")
                    ],
                ],
            ),
            (
                "xiaohongshu",
                ["comments"],
                operations(
                    "unknown",
                    "authorized_session",
                    "公开页面未证明搜索、根评论或回复结构。",
                ),
            ),
            (
                "douyin",
                ["comments"],
                operations(
                    "unknown",
                    "authorized_session",
                    "公开应用壳未证明搜索、根评论或回复结构。",
                ),
            ),
            (
                "bluesky",
                ["supplement"],
                [
                    capability(
                        "search_posts",
                        "authorization_required",
                        "public_api",
                        "历史实测搜索返回403，不能作为主线发现入口。",
                    ),
                    *[
                        capability(
                            operation,
                            "supported",
                            "public_api",
                            "公开线程读取通过有界探测，仍未接入持久化流水线。",
                        )
                        for operation in ("fetch_post", "list_comments", "list_replies")
                    ],
                ],
            ),
        )
        return [
            SourceView.model_validate(
                {
                    "id": source,
                    "roles": roles,
                    "pipeline": "not_connected",
                    "eligible_for_collection": False,
                    "operations": source_operations,
                }
            )
            for source, roles, source_operations in rows
        ]

    def preview(self, data: SearchInput) -> QueryPreview:
        return QueryPreview(
            query=data.keyword, since=data.since, until=data.until, limit=data.limit
        )
