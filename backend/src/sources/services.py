from datetime import date

from sources.schemas import (
    QueryPreview,
    QueryPreviewInput,
    QueryRuleExecution,
    QuerySpec,
    SourceName,
    SourceOperationCapability,
    SourceQueryPreview,
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

    def preview(self, data: QueryPreviewInput) -> QueryPreview:
        catalog = {source.id: source for source in self.catalog()}
        previews: list[SourceQueryPreview] = []
        terms = list(dict.fromkeys([*data.query_spec.include_any, *data.query_spec.aliases]))
        for source_id in data.source_ids:
            source = catalog[source_id]
            search = next(
                operation
                for operation in source.operations
                if operation.operation == "search_posts"
            )
            compilable = search.support in {"supported", "authorization_required"}
            queries = terms if compilable else []
            rules = [
                QueryRuleExecution(
                    rule="include_any", mode="native" if compilable else "unsupported"
                ),
                QueryRuleExecution(
                    rule="include_all", mode="local_filter" if compilable else "unsupported"
                ),
                QueryRuleExecution(
                    rule="exclude", mode="local_filter" if compilable else "unsupported"
                ),
                QueryRuleExecution(rule="aliases", mode="native" if compilable else "unsupported"),
            ]
            previews.append(
                SourceQueryPreview(
                    source=source_id,
                    support=search.support,
                    queries=queries,
                    rules=rules,
                    estimated_requests=len(queries),
                )
            )
        return QueryPreview(
            since=data.since,
            until=data.until,
            sources=previews,
            estimated_requests=sum(source.estimated_requests for source in previews),
        )

    def activation_issues(self, source_ids: list[SourceName]) -> list[SourceName]:
        catalog = {source.id: source for source in self.catalog()}
        return [
            source_id for source_id in source_ids if not catalog[source_id].eligible_for_collection
        ]

    def request_estimate(self, query_spec: QuerySpec, source_ids: list[SourceName]) -> int:
        terms = set([*query_spec.include_any, *query_spec.aliases])
        catalog = {source.id: source for source in self.catalog()}
        return sum(
            len(terms)
            for source_id in source_ids
            if next(
                operation
                for operation in catalog[source_id].operations
                if operation.operation == "search_posts"
            ).support
            in {"supported", "authorization_required"}
        )
