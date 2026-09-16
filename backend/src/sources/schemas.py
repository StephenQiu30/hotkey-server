import re
from datetime import UTC, date, datetime
from typing import Literal, Self

from pydantic import AwareDatetime, BaseModel, ConfigDict, Field, model_validator

from core.schemas import Input

POST_URI = r"^at://did:[a-z0-9]+:[a-zA-Z0-9._:%-]+/app\.bsky\.feed\.post/[a-zA-Z0-9._~:-]+$"
BILIBILI_BVID = r"^BV[1-9A-HJ-NP-Za-km-z]{10}$"

SourceName = Literal["x", "bilibili", "weibo", "xiaohongshu", "douyin", "bluesky"]
SourceOperation = Literal[
    "search_posts", "fetch_post", "list_comments", "list_replies", "fetch_thread"
]
SourceRecoveryAction = Literal[
    "refresh_authorization",
    "wait_for_rate_limit",
    "check_source_availability",
    "update_adapter",
    "review_run",
]


class SourceOperationRuntime(BaseModel):
    status: Literal["unobserved", "healthy", "degraded"]
    last_success_at: AwareDatetime | None = None
    last_failure_at: AwareDatetime | None = None
    last_failure_code: str | None = Field(default=None, min_length=1, max_length=80)
    recovery_action: SourceRecoveryAction | None = None

    @model_validator(mode="after")
    def valid_runtime(self) -> Self:
        if (self.last_failure_at is None) != (self.last_failure_code is None):
            raise ValueError("failure time and code must be provided together")
        if self.status == "unobserved" and any(
            value is not None
            for value in (
                self.last_success_at,
                self.last_failure_at,
                self.last_failure_code,
                self.recovery_action,
            )
        ):
            raise ValueError("unobserved runtime cannot contain observations")
        if self.status == "healthy" and self.last_success_at is None:
            raise ValueError("healthy runtime requires a successful observation")
        if self.status == "degraded" and (
            self.last_failure_at is None
            or self.last_failure_code is None
            or self.recovery_action is None
        ):
            raise ValueError("degraded runtime requires failure details and recovery action")
        if self.status != "degraded" and self.recovery_action is not None:
            raise ValueError("only degraded runtime can expose a recovery action")
        return self


class SourceOperationCapability(BaseModel):
    operation: Literal["search_posts", "fetch_post", "list_comments", "list_replies"]
    support: Literal["unknown", "supported", "unsupported", "authorization_required"]
    rights: Literal["unknown", "allowed", "denied"]
    pipeline: Literal["not_connected", "connected", "degraded", "paused"]
    eligible_for_collection: bool
    requires_operations: list[
        Literal["search_posts", "fetch_post", "list_comments", "list_replies"]
    ]
    access_mode: Literal["public_web", "public_api", "official_paid_api", "authorized_session"]
    content_purchase_cost: Literal[0] = 0
    verified_at: date
    evidence_ref: str = Field(min_length=1, max_length=255)
    note: str = Field(min_length=1, max_length=500)
    runtime: SourceOperationRuntime


class SourceView(BaseModel):
    id: SourceName
    roles: list[Literal["discovery", "comments", "supplement"]]
    operations: list[SourceOperationCapability]


class SearchInput(Input):
    keyword: str = Field(min_length=1, max_length=100)
    since: AwareDatetime
    until: AwareDatetime
    limit: int = Field(default=20, ge=1, le=100)
    cursor: str | None = Field(default=None, min_length=1, max_length=2048)

    @model_validator(mode="after")
    def valid_window(self) -> Self:
        if self.since >= self.until:
            raise ValueError("since must precede until")
        self.since = self.since.astimezone(UTC)
        self.until = self.until.astimezone(UTC)
        return self


class ThreadInput(Input):
    uri: str = Field(pattern=POST_URI, max_length=1024)
    depth: int = Field(default=2, ge=0, le=3)
    max_nodes: int = Field(default=100, ge=1, le=200)


class BilibiliSearchInput(Input):
    keyword: str = Field(min_length=1, max_length=100)
    page: int = Field(default=1, ge=1, le=50)
    limit: int = Field(default=20, ge=1, le=20)


class BilibiliPostInput(Input):
    bvid: str = Field(pattern=BILIBILI_BVID, max_length=12)


class BilibiliCommentsInput(Input):
    aid: int = Field(gt=0)
    cursor: int = Field(default=0, ge=0)
    limit: int = Field(default=20, ge=1, le=20)


class BilibiliRepliesInput(Input):
    aid: int = Field(gt=0)
    root_id: int = Field(gt=0)
    page: int = Field(default=1, ge=1)
    limit: int = Field(default=20, ge=1, le=20)


class QuerySpec(BaseModel):
    include_any: list[str] = Field(min_length=1, max_length=20)
    include_all: list[str] = Field(default_factory=list, max_length=10)
    exclude: list[str] = Field(default_factory=list, max_length=20)
    aliases: list[str] = Field(default_factory=list, max_length=20)

    @classmethod
    def _clean_terms(cls, values: list[str]) -> list[str]:
        cleaned = [value.strip() for value in values]
        if any(not value or len(value) > 100 for value in cleaned):
            raise ValueError("query terms must contain 1 to 100 characters")
        return list(dict.fromkeys(cleaned))

    @model_validator(mode="after")
    def valid_terms(self) -> Self:
        self.include_any = self._clean_terms(self.include_any)
        self.include_all = self._clean_terms(self.include_all)
        self.exclude = self._clean_terms(self.exclude)
        self.aliases = self._clean_terms(self.aliases)
        return self


class QueryPreviewInput(Input):
    query_spec: QuerySpec
    source_ids: list[SourceName] = Field(min_length=1, max_length=6)
    since: AwareDatetime
    until: AwareDatetime

    @model_validator(mode="after")
    def valid_preview(self) -> Self:
        if self.since >= self.until:
            raise ValueError("since must precede until")
        self.since = self.since.astimezone(UTC)
        self.until = self.until.astimezone(UTC)
        self.source_ids = list(dict.fromkeys(self.source_ids))
        return self


class QueryRuleExecution(BaseModel):
    rule: Literal["include_any", "include_all", "exclude", "aliases"]
    mode: Literal["native", "local_filter", "unsupported"]


class SourceQueryPreview(BaseModel):
    source: SourceName
    operation: Literal["search_posts"] = "search_posts"
    support: Literal["unknown", "supported", "unsupported", "authorization_required"]
    queries: list[str]
    rules: list[QueryRuleExecution]
    estimated_requests: int = Field(ge=0)
    content_purchase_cost: Literal[0] = 0
    pipeline_connected: bool


class QueryPreview(BaseModel):
    since: datetime
    until: datetime
    sources: list[SourceQueryPreview]
    estimated_requests: int = Field(ge=0)
    content_purchase_cost: Literal[0] = 0
    network_accessed: Literal[False] = False


class CollectionPageInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)
    source: SourceName
    operation: Literal["search_posts", "fetch_post", "list_comments", "list_replies"]
    request_value: str = Field(min_length=1, max_length=100)
    cursor: str | None = Field(default=None, max_length=20, pattern=r"^[1-9][0-9]*$")
    since: AwareDatetime
    until: AwareDatetime
    limit: int = Field(default=1, ge=1, le=20)

    @model_validator(mode="after")
    def valid_request(self) -> Self:
        if self.since >= self.until:
            raise ValueError("since must precede until")
        if self.cursor is not None and self.operation not in {"list_comments", "list_replies"}:
            raise ValueError("cursor is only supported for comment operations")
        if self.operation == "fetch_post" and not re.fullmatch(
            rf"bvid:{BILIBILI_BVID[1:-1]}", self.request_value
        ):
            raise ValueError("fetch_post requires a bvid reference")
        if self.operation == "list_comments" and not re.fullmatch(
            r"aid:[1-9][0-9]*", self.request_value
        ):
            raise ValueError("list_comments requires an aid reference")
        if self.operation == "list_replies" and not re.fullmatch(
            r"aid:[1-9][0-9]*/root:[1-9][0-9]*", self.request_value
        ):
            raise ValueError("list_replies requires an aid and root reference")
        expected_limit = 20 if self.operation in {"list_comments", "list_replies"} else 1
        if self.limit != expected_limit:
            raise ValueError(f"{self.operation} requires limit {expected_limit}")
        object.__setattr__(self, "since", self.since.astimezone(UTC))
        object.__setattr__(self, "until", self.until.astimezone(UTC))
        return self


class SocialObject(BaseModel):
    model_config = ConfigDict(extra="forbid")
    provider_namespace: str = Field(min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9._:-]+$")
    external_id: str = Field(min_length=1, max_length=1024)
    kind: Literal["post", "comment", "reply"]
    text: str = Field(max_length=10000)
    author_id: str = Field(min_length=1, max_length=1024)
    created_at: AwareDatetime
    root_id: str = Field(min_length=1, max_length=1024)
    parent_id: str | None = Field(default=None, min_length=1, max_length=1024)
    reply_count: int | None = Field(default=None, ge=0)
    canonical_url: str | None = Field(default=None, min_length=1, max_length=2048)

    @model_validator(mode="after")
    def valid_relationship(self) -> Self:
        if self.kind == "post" and (self.parent_id is not None or self.root_id != self.external_id):
            raise ValueError("post must be its own root and cannot have a parent")
        if self.kind == "reply" and self.parent_id is None:
            raise ValueError("reply requires a parent")
        return self


class SourceReference(BaseModel):
    external_id: str
    canonical_url: str


class UnavailableObject(BaseModel):
    external_id: str
    reason: Literal["not_found", "blocked"]


class SourceResult(BaseModel):
    source: SourceName = "bluesky"
    adapter_version: str = Field(default="bluesky-v1", min_length=1, max_length=50)
    operation: SourceOperation
    status: Literal["ok", "empty", "partial", "failed"]
    code: str | None = None
    observed_at: datetime
    references: list[SourceReference] = Field(default_factory=list)
    items: list[SocialObject] = Field(default_factory=list)
    unavailable: list[UnavailableObject] = Field(default_factory=list)
    cursor: str | None = None
    total: int | None = None
    coverage: Literal["unknown"] = "unknown"
    response_bytes: int = 0
    response_sha256: str | None = None
    http_status: int | None = None
    retry_after_seconds: int | None = None
