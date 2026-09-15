from datetime import UTC, datetime
from typing import Literal, Self
from uuid import UUID

from pydantic import AwareDatetime, BaseModel, Field, model_validator

from core.schemas import Input
from sources.schemas import SourceName


class AnalysisRunInput(Input):
    expected_event_revision: int = Field(ge=1)
    since: AwareDatetime
    until: AwareDatetime
    cutoff: AwareDatetime
    max_items: int = Field(default=24, ge=1, le=100)

    @model_validator(mode="after")
    def valid_window(self) -> Self:
        self.since = self.since.astimezone(UTC)
        self.until = self.until.astimezone(UTC)
        self.cutoff = self.cutoff.astimezone(UTC)
        if self.since >= self.until:
            raise ValueError("since must precede until")
        if self.until > self.cutoff:
            raise ValueError("cutoff must include the complete sample window")
        return self


class AnalysisLabelInput(Input):
    topic: str = Field(default="", max_length=80)
    target: str = Field(default="", max_length=160)
    sentiment: Literal["positive", "negative", "neutral", "mixed", "unknown"]
    stance: Literal["support", "oppose", "neutral", "mixed", "unknown"]
    request: str = Field(default="", max_length=500)
    abstained: bool = False
    citation_content_version_ids: list[UUID] = Field(default_factory=list, max_length=3)

    @model_validator(mode="after")
    def valid_label(self) -> Self:
        if len(set(self.citation_content_version_ids)) != len(self.citation_content_version_ids):
            raise ValueError("citation_content_version_ids must be unique")
        if self.abstained:
            if self.sentiment != "unknown" or self.stance != "unknown":
                raise ValueError("abstained labels require unknown sentiment and stance")
            if any((self.topic.strip(), self.target.strip(), self.request.strip())):
                raise ValueError("abstained labels cannot assert a topic, target or request")
        elif not (self.topic.strip() and self.target.strip() and self.citation_content_version_ids):
            raise ValueError("non-abstained labels require topic, target and a citation")
        self.topic = self.topic.strip()
        self.target = self.target.strip()
        self.request = self.request.strip()
        return self


class AnalysisContextView(BaseModel):
    role: Literal["sample", "parent", "root"]
    content_id: UUID
    content_version_id: UUID
    text: str | None
    text_sha256: str
    canonical_url: str | None
    available: bool


class AnalysisLabelView(BaseModel):
    id: UUID
    topic: str
    target: str
    sentiment: Literal["positive", "negative", "neutral", "mixed", "unknown"]
    stance: Literal["support", "oppose", "neutral", "mixed", "unknown"]
    request: str
    abstained: bool
    citation_content_version_ids: list[UUID]
    label_source: Literal["manual"]
    schema_version: Literal["analysis-label-v1"]
    created_at: datetime


class AnalysisSampleView(BaseModel):
    id: UUID
    position: int = Field(ge=1)
    source: SourceName
    kind: Literal["comment", "reply"]
    root_external_id: str
    published_at: datetime
    time_bucket_start: datetime
    ordering_origin: Literal["provider_default"]
    selection_reason: Literal["stratum_round_robin"]
    contexts: list[AnalysisContextView]
    label: AnalysisLabelView | None


class AnalysisViewpoint(BaseModel):
    topic: str
    target: str
    sentiment: Literal["positive", "negative", "neutral", "mixed", "unknown"]
    stance: Literal["support", "oppose", "neutral", "mixed", "unknown"]
    request: str
    sample_count: int = Field(ge=1)
    citations: list[AnalysisContextView] = Field(min_length=1)


class AnalysisComposition(BaseModel):
    platforms: dict[str, int]
    roots: int = Field(ge=0)
    time_buckets: dict[str, int]
    ordering_origins: dict[str, int]


class AnalysisRunSummary(BaseModel):
    id: UUID
    event_id: UUID
    event_revision: int
    status: Literal["pending", "succeeded", "stale"]
    manifest_sha256: str
    sample_count: int
    labeled_count: int
    abstained_count: int
    created_at: datetime


class AnalysisRunPage(BaseModel):
    items: list[AnalysisRunSummary]


class AnalysisRunView(BaseModel):
    id: UUID
    event_id: UUID
    event_revision: int
    status: Literal["pending", "succeeded", "stale"]
    method: Literal["manual_baseline"]
    analyzer_id: Literal["manual"]
    prompt_version: Literal["none"]
    label_schema_version: Literal["analysis-label-v1"]
    sampling_policy_version: Literal["event-comment-stratified-v1"]
    manifest_sha256: str
    since: datetime
    until: datetime
    cutoff: datetime
    max_items: int
    sample_count: int
    labeled_count: int
    valid_labeled_count: int
    abstained_count: int
    pending_count: int
    token_budget: Literal[0]
    input_tokens: Literal[0]
    output_tokens: Literal[0]
    composition: AnalysisComposition
    samples: list[AnalysisSampleView]
    viewpoints: list[AnalysisViewpoint]
    created_at: datetime
    updated_at: datetime
    recomputed_from_manifest: bool = False


class ControlledCommentStatistics(BaseModel):
    event_id: UUID
    event_revision: int = Field(ge=1)
    since: datetime
    until: datetime
    rule_version: Literal["event-comment-count-v1"] = "event-comment-count-v1"
    total: int = Field(ge=0)
    by_source: dict[str, int]
    by_kind: dict[str, int]
