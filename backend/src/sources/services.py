from collections.abc import Iterable
from datetime import date

from pydantic import ValidationError

from core.config import Settings
from sources.schemas import (
    BilibiliPostInput,
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
SOURCE_IDS: tuple[SourceName, ...] = (
    "x",
    "bilibili",
    "weibo",
    "xiaohongshu",
    "douyin",
    "bluesky",
)
CATALOG_OPERATIONS = ("search_posts", "fetch_post", "list_comments", "list_replies")
KNOWN_SOURCE_OPERATIONS = frozenset(
    f"{source}.{operation}" for source in SOURCE_IDS for operation in CATALOG_OPERATIONS
)
PERSISTENT_SOURCE_OPERATIONS = frozenset(
    {"bilibili.search_posts", "bilibili.fetch_post", "bilibili.list_comments"}
)
DISCOVERY_REFERENCE_LIMIT = 1


def capability(
    operation: str,
    support: str,
    access_mode: str,
    note: str,
    requires_operations: tuple[str, ...] = (),
) -> SourceOperationCapability:
    return SourceOperationCapability.model_validate(
        {
            "operation": operation,
            "support": support,
            "rights": "unknown",
            "pipeline": "not_connected",
            "eligible_for_collection": False,
            "requires_operations": list(requires_operations),
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
    def __init__(
        self,
        *,
        rights_allowed: Iterable[str] = (),
        pipelines_connected: Iterable[str] = (),
    ):
        self.rights_allowed = frozenset(rights_allowed)
        self.pipelines_connected = frozenset(pipelines_connected)
        unknown = (self.rights_allowed | self.pipelines_connected) - KNOWN_SOURCE_OPERATIONS
        if unknown:
            raise ValueError("unknown source operation: " + sorted(unknown)[0])
        unimplemented = self.pipelines_connected - PERSISTENT_SOURCE_OPERATIONS
        if unimplemented:
            raise ValueError(
                "persistent source operation is not implemented: " + sorted(unimplemented)[0]
            )

    @classmethod
    def from_settings(cls, settings: Settings) -> "SourceService":
        return cls(
            rights_allowed=settings.source_rights_allowed,
            pipelines_connected=settings.source_pipelines_connected,
        )

    def _operation(
        self, source: SourceName, value: SourceOperationCapability
    ) -> SourceOperationCapability:
        key = f"{source}.{value.operation}"
        rights = (
            "allowed" if key in self.rights_allowed and value.rights != "denied" else value.rights
        )
        pipeline = "connected" if key in self.pipelines_connected else "not_connected"
        return value.model_copy(
            update={
                "rights": rights,
                "pipeline": pipeline,
                "eligible_for_collection": (
                    value.support == "supported" and rights == "allowed" and pipeline == "connected"
                ),
            }
        )

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
                [
                    capability(
                        "search_posts",
                        "supported",
                        "public_web",
                        "无Cookie小样本可解析；正文入箱还需要帖子详情操作。",
                        ("fetch_post",),
                    ),
                    *[
                        capability(
                            operation,
                            "supported",
                            "public_web",
                            "无Cookie小样本可解析；长期保存与派生分析权限仍待核对。",
                            ("list_comments",) if operation == "fetch_post" else (),
                        )
                        for operation in ("fetch_post", "list_comments", "list_replies")
                    ],
                ],
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
        result = []
        for source, roles, source_operations in rows:
            admitted = [self._operation(source, operation) for operation in source_operations]
            by_operation: dict[str, SourceOperationCapability] = {
                operation.operation: operation for operation in admitted
            }

            def eligible(
                operation: SourceOperationCapability,
                path: frozenset[str],
                capabilities: dict[str, SourceOperationCapability] = by_operation,
            ) -> bool:
                if operation.operation in path or not operation.eligible_for_collection:
                    return False
                return all(
                    eligible(capabilities[required], path | {operation.operation}, capabilities)
                    for required in operation.requires_operations
                )

            resolved = [
                operation.model_copy(
                    update={"eligible_for_collection": eligible(operation, frozenset())}
                )
                for operation in admitted
            ]
            result.append(SourceView(id=source, roles=roles, operations=resolved))
        return result

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
                    estimated_requests=len(queries)
                    * self._request_cost(source.operations, "search_posts"),
                    pipeline_connected=search.pipeline == "connected",
                )
            )
        return QueryPreview(
            since=data.since,
            until=data.until,
            sources=previews,
            estimated_requests=sum(source.estimated_requests for source in previews),
        )

    def activation_issues(
        self, source_ids: list[SourceName], operation: str = "search_posts"
    ) -> list[SourceName]:
        catalog = {source.id: source for source in self.catalog()}
        return [
            source_id
            for source_id in source_ids
            if not any(
                capability.operation == operation and capability.eligible_for_collection
                for capability in catalog[source_id].operations
            )
        ]

    @staticmethod
    def request_value_is_valid(source: SourceName, operation: str, value: str) -> bool:
        if source != "bilibili":
            return False
        try:
            if operation == "fetch_post" and value.startswith("bvid:"):
                BilibiliPostInput(bvid=value.removeprefix("bvid:"))
            elif operation == "list_comments" and value.startswith("aid:"):
                identity = value.removeprefix("aid:")
                if not identity.isascii() or not identity.isdigit() or int(identity) <= 0:
                    return False
            else:
                return False
        except ValidationError:
            return False
        return True

    @staticmethod
    def _request_cost(
        operations: list[SourceOperationCapability],
        operation_name: str,
        path: frozenset[str] = frozenset(),
    ) -> int:
        if operation_name in path:
            raise ValueError("cyclic source operation dependency")
        operation = next(item for item in operations if item.operation == operation_name)
        return 1 + DISCOVERY_REFERENCE_LIMIT * sum(
            SourceService._request_cost(operations, required, path | {operation_name})
            for required in operation.requires_operations
        )

    def request_estimate(self, query_spec: QuerySpec, source_ids: list[SourceName]) -> int:
        terms = set([*query_spec.include_any, *query_spec.aliases])
        catalog = {source.id: source for source in self.catalog()}
        return sum(
            len(terms) * self._request_cost(catalog[source_id].operations, operation.operation)
            for source_id in source_ids
            if (
                operation := next(
                    operation
                    for operation in catalog[source_id].operations
                    if operation.operation == "search_posts"
                )
            ).support
            in {"supported", "authorization_required"}
        )
