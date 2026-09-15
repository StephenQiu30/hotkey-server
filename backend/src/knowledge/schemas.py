from datetime import UTC, datetime
from typing import Annotated, Literal, Self
from uuid import UUID

from pydantic import AwareDatetime, BaseModel, Field, model_validator

from analysis.schemas import ControlledCommentStatistics
from core.schemas import Input


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


class CommentCountQuestion(Input):
    kind: Literal["comment_count"]
    question: str = Field(min_length=2, max_length=300)
    event_id: UUID
    since: AwareDatetime
    until: AwareDatetime

    @model_validator(mode="after")
    def valid_window(self) -> Self:
        self.since = self.since.astimezone(UTC)
        self.until = self.until.astimezone(UTC)
        if self.since >= self.until:
            raise ValueError("since must precede until")
        return self


class EvidenceQuestion(Input):
    kind: Literal["evidence"]
    question: str = Field(min_length=2, max_length=300)
    event_id: UUID | None = None
    mode: Literal["exact_substring", "semantic"] = "exact_substring"
    limit: int = Field(default=3, ge=1, le=5)


KnowledgeQuestion = Annotated[
    CommentCountQuestion | EvidenceQuestion,
    Field(discriminator="kind"),
]


class KnowledgeAnswer(BaseModel):
    kind: Literal["comment_count", "evidence"]
    status: Literal["answered", "unknown"]
    question: str
    answer: str
    method: Literal["controlled_statistics_v1", "deterministic_retrieval_v1"]
    statistics: ControlledCommentStatistics | None = None
    knowledge_entry_ids: list[UUID] = Field(default_factory=list)
    citations: list[KnowledgeCitationView] = Field(default_factory=list)
    unknown_reason: Literal["no_supported_evidence"] | None = None
