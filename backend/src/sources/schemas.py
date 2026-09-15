from datetime import UTC, date, datetime
from typing import Literal, Self

from pydantic import AwareDatetime, BaseModel, Field, model_validator

from core.schemas import Input

POST_URI = r"^at://did:[a-z0-9]+:[a-zA-Z0-9._:%-]+/app\.bsky\.feed\.post/[a-zA-Z0-9._~:-]+$"
BILIBILI_BVID = r"^BV[1-9A-HJ-NP-Za-km-z]{10}$"

SourceName = Literal["x", "bilibili", "weibo", "xiaohongshu", "douyin", "bluesky"]
SourceOperation = Literal[
    "search_posts", "fetch_post", "list_comments", "list_replies", "fetch_thread"
]


class SourceOperationCapability(BaseModel):
    operation: Literal["search_posts", "fetch_post", "list_comments", "list_replies"]
    support: Literal["unknown", "supported", "unsupported", "authorization_required"]
    rights: Literal["unknown", "allowed", "denied"]
    access_mode: Literal["public_web", "public_api", "official_paid_api", "authorized_session"]
    content_purchase_cost: Literal[0] = 0
    verified_at: date
    evidence_ref: str = Field(min_length=1, max_length=255)
    note: str = Field(min_length=1, max_length=500)


class SourceView(BaseModel):
    id: SourceName
    roles: list[Literal["discovery", "comments", "supplement"]]
    pipeline: Literal["not_connected", "connected", "degraded", "paused"]
    eligible_for_collection: bool
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


class QueryPreview(BaseModel):
    source: Literal["bluesky"] = "bluesky"
    operation: Literal["search_posts"] = "search_posts"
    adapter_version: Literal["bluesky-v1"] = "bluesky-v1"
    query: str
    since: datetime
    until: datetime
    sort: Literal["latest"] = "latest"
    limit: int
    semantics: Literal["provider_native_text"] = "provider_native_text"
    coverage: Literal["unknown"] = "unknown"
    pipeline_connected: Literal[False] = False


class SocialObject(BaseModel):
    external_id: str
    kind: Literal["post", "comment", "reply"]
    text: str
    author_id: str
    created_at: AwareDatetime
    root_id: str
    parent_id: str | None = None
    reply_count: int | None = None
    canonical_url: str | None = None


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
