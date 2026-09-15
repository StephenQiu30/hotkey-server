from datetime import UTC, datetime
from typing import Literal, Self
from uuid import UUID

from pydantic import AwareDatetime, BaseModel, ConfigDict, Field, model_validator

from core.schemas import Input
from sources.schemas import SourceName, SourceResult


class CollectionRunInput(Input):
    monitor_id: UUID
    expected_version: int = Field(ge=1)
    source: SourceName
    operation: Literal["search_posts"] = "search_posts"
    query_variant: str = Field(min_length=1, max_length=100)
    since: AwareDatetime
    until: AwareDatetime
    idempotency_key: str = Field(min_length=1, max_length=128, pattern=r"^[a-zA-Z0-9_.:-]+$")
    policy_version: str = Field(min_length=1, max_length=64)
    retention_days: int = Field(ge=1, le=365)

    @model_validator(mode="after")
    def valid_window(self) -> Self:
        if self.since >= self.until:
            raise ValueError("since must precede until")
        self.since = self.since.astimezone(UTC)
        self.until = self.until.astimezone(UTC)
        return self


class CollectionRunView(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: UUID
    job_id: UUID
    monitor_version_id: UUID
    source: SourceName
    operation: Literal["search_posts"]
    query_variant: str
    retention_days: int
    state: Literal["queued", "running", "completed", "failed", "cancelled"]
    outcome: Literal["ok", "empty", "partial", "failed"] | None
    fencing_token: int
    pages_count: int
    items_count: int
    bytes_count: int
    stop_reason: str | None
    created_at: datetime
    completed_at: datetime | None


class CollectionRunRequest(Input):
    expected_version: int = Field(ge=1)
    source: SourceName
    operation: Literal["search_posts"] = "search_posts"
    query_variant: str = Field(min_length=1, max_length=100)
    since: AwareDatetime
    until: AwareDatetime
    policy_version: str = Field(min_length=1, max_length=64)
    retention_days: int = Field(ge=1, le=365)

    @model_validator(mode="after")
    def valid_window(self) -> Self:
        if self.since >= self.until:
            raise ValueError("since must precede until")
        self.since = self.since.astimezone(UTC)
        self.until = self.until.astimezone(UTC)
        return self


class CollectionExecutionInput(BaseModel):
    model_config = ConfigDict(frozen=True)
    run_id: UUID
    job_id: UUID
    fencing_token: int = Field(ge=1)
    source: SourceName
    operation: Literal["search_posts"]
    query_variant: str
    since: datetime
    until: datetime
    policy_version: str
    retention_days: int = Field(ge=1, le=365)


class CollectionLease(BaseModel):
    model_config = ConfigDict(frozen=True)
    run_id: UUID
    fencing_token: int = Field(ge=1)


class PageCommitInput(Input):
    run_id: UUID
    fencing_token: int = Field(ge=1)
    page_key: str = Field(min_length=1, max_length=128, pattern=r"^[a-zA-Z0-9_.:-]+$")
    request_fingerprint: str = Field(pattern=r"^[a-f0-9]{64}$")
    media_type: Literal["application/json", "text/html"]
    payload: bytes = Field(max_length=2 * 1024 * 1024)
    retention_until: AwareDatetime
    policy_version: str = Field(min_length=1, max_length=64)
    result: SourceResult

    @model_validator(mode="after")
    def normalize_retention(self) -> Self:
        self.retention_until = self.retention_until.astimezone(UTC)
        if self.retention_until <= self.result.observed_at:
            raise ValueError("retention_until must follow observed_at")
        return self


class PageCommitView(BaseModel):
    run_id: UUID
    checkpoint_id: UUID
    raw_page_id: UUID
    item_count: int
    new_content_count: int
    new_version_count: int
    duplicate: bool
