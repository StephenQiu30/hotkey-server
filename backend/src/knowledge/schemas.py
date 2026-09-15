from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, Field


class KnowledgeCitationView(BaseModel):
    content_version_id: UUID
    text_sha256: str
    text: str | None
    canonical_url: str | None
    available: bool


class KnowledgeEntryView(BaseModel):
    id: UUID
    event_id: UUID
    source_analysis_run_id: UUID
    entry_type: Literal["analysis_snapshot"]
    version: int = Field(ge=1)
    title: str
    body: str
    analysis_manifest_sha256: str
    semantic_index_state: Literal["pending", "ready", "failed", "stale", "deleted"]
    similarity: float | None = Field(default=None, ge=-1, le=1)
    stale: bool
    citations: list[KnowledgeCitationView] = Field(min_length=1)
    created_at: datetime
    updated_at: datetime


class KnowledgePage(BaseModel):
    query_mode: Literal["exact_substring", "semantic"] = "exact_substring"
    query: str
    items: list[KnowledgeEntryView]
