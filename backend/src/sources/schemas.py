from datetime import UTC, datetime
from typing import Literal, Self

from pydantic import AwareDatetime, BaseModel, Field, model_validator

from core.schemas import Input

POST_URI = r"^at://did:[a-z0-9]+:[a-zA-Z0-9._:%-]+/app\.bsky\.feed\.post/[a-zA-Z0-9._~:-]+$"


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


class UnavailableObject(BaseModel):
    external_id: str
    reason: Literal["not_found", "blocked"]


class SourceResult(BaseModel):
    source: Literal["bluesky"] = "bluesky"
    adapter_version: Literal["bluesky-v1"] = "bluesky-v1"
    operation: Literal["search_posts", "thread"]
    status: Literal["ok", "empty", "partial", "failed"]
    code: str | None = None
    observed_at: datetime
    items: list[SocialObject] = Field(default_factory=list)
    unavailable: list[UnavailableObject] = Field(default_factory=list)
    cursor: str | None = None
    total: int | None = None
    coverage: Literal["unknown"] = "unknown"
    response_bytes: int = 0
    response_sha256: str | None = None
    http_status: int | None = None
    retry_after_seconds: int | None = None
