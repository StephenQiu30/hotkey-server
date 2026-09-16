from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import AwareDatetime, BaseModel, Field

from core.schemas import Input
from sources.schemas import SourceName


class InboxItem(BaseModel):
    id: UUID
    source: SourceName
    provider_namespace: str
    external_id: str
    kind: Literal["post", "comment", "reply"]
    root_external_id: str
    parent_external_id: str | None
    relation_status: Literal["root", "unresolved", "resolved"]
    text: str
    version: int
    canonical_url: str | None
    published_at: datetime
    first_seen_at: datetime
    last_seen_at: datetime
    reply_count: int | None
    monitor_titles: list[str]


class InboxPage(BaseModel):
    items: list[InboxItem]
    next_cursor: str | None


class ContentWithdrawalInput(Input):
    reason: Literal["deleted", "purpose_revoked"]


class ContentWithdrawalView(BaseModel):
    id: UUID
    visibility: Literal["unavailable", "deleted"]
    affected_knowledge_entries: int


class ContentWithdrawalManifestEntry(Input):
    source: SourceName
    provider_namespace: str = Field(min_length=1, max_length=100)
    external_id: str = Field(min_length=1, max_length=1024)
    visibility: Literal["unavailable", "deleted"]
    effective_at: AwareDatetime


class ContentWithdrawalManifest(Input):
    schema_version: Literal["content-withdrawal-manifest-v1"]
    generated_at: AwareDatetime
    entries: list[ContentWithdrawalManifestEntry] = Field(max_length=100_000)


class ContentWithdrawalReplayView(BaseModel):
    applied: int
    missing: int
