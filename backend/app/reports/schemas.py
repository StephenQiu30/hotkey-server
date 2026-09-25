from __future__ import annotations

from datetime import datetime, timedelta
from enum import StrEnum
from typing import Self
from urllib.parse import urlsplit
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator


class ReportKind(StrEnum):
    DAILY = "daily"
    WEEKLY = "weekly"


class ReportStatus(StrEnum):
    DRAFT = "draft"
    FINAL = "final"


class ReportGenerator(StrEnum):
    TEMPLATE = "template"
    MODEL = "model"


class ReportPeriod(StrEnum):
    CURRENT = "current"
    PREVIOUS = "previous"


class AnnotationState(StrEnum):
    MISSING = "missing"
    ANNOTATED = "annotated"
    UNANALYZED = "unanalyzed"


class ReportSentiment(StrEnum):
    POSITIVE = "positive"
    NEUTRAL = "neutral"
    NEGATIVE = "negative"


class SourceCoverageStatus(StrEnum):
    SUCCEEDED = "succeeded"
    PARTIAL = "partial"
    FAILED = "failed"
    INCOMPLETE = "incomplete"
    MISSING = "missing"


class ReportMetricInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    like_count: int = Field(default=0, ge=0)
    comment_count: int = Field(default=0, ge=0)
    repost_count: int = Field(default=0, ge=0)
    view_count: int = Field(default=0, ge=0)
    play_count: int = Field(default=0, ge=0)
    danmaku_count: int = Field(default=0, ge=0)

    @property
    def interaction_count(self) -> int:
        return self.like_count + self.comment_count + self.repost_count + self.danmaku_count


class ReportPostInput(BaseModel):
    """One exact post version and annotation selected at the report cutoff."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    period: ReportPeriod
    content_id: UUID
    content_version_id: UUID
    observation_id: UUID
    annotation_id: UUID | None
    source_key: str = Field(min_length=1, max_length=64)
    title: str | None = Field(default=None, max_length=2_000)
    body: str | None = Field(default=None, max_length=100_000)
    url: str | None = Field(default=None, max_length=2_048)
    published_at: datetime | None
    first_observed_at: datetime
    occurred_at: datetime
    annotation_state: AnnotationState
    relevant: bool | None
    sentiment: ReportSentiment | None
    summary: str | None = Field(default=None, max_length=60)
    relevance_reason: str | None = Field(default=None, max_length=500)
    viewpoints: tuple[str, ...] = Field(default=(), max_length=5)
    metrics: ReportMetricInput

    @field_validator("url")
    @classmethod
    def validate_url(cls, value: str | None) -> str | None:
        if value is None:
            return None
        parsed = urlsplit(value)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ValueError("report source URL must use HTTP or HTTPS")
        return value

    @field_validator("published_at", "first_observed_at", "occurred_at")
    @classmethod
    def require_aware_time(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.utcoffset() is None:
            raise ValueError("report input times must be timezone-aware")
        return value

    @model_validator(mode="after")
    def validate_annotation(self) -> Self:
        if self.annotation_state is AnnotationState.MISSING:
            if self.annotation_id is not None or self.relevant is not None:
                raise ValueError("missing annotations cannot carry annotation output")
        elif self.annotation_id is None:
            raise ValueError("persisted annotation states require an annotation id")
        if self.annotation_state is AnnotationState.ANNOTATED:
            if self.relevant is None or self.summary is None or self.relevance_reason is None:
                raise ValueError("annotated report inputs require complete output")
            if self.relevant and self.sentiment is None:
                raise ValueError("relevant report inputs require sentiment")
            if not self.relevant and self.sentiment is not None:
                raise ValueError("irrelevant report inputs cannot carry sentiment")
        elif any(
            item is not None
            for item in (self.relevant, self.sentiment, self.summary, self.relevance_reason)
        ):
            raise ValueError("unanalyzed report inputs cannot carry annotation output")
        return self


class ReportCommentInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    period: ReportPeriod
    post_content_id: UUID
    content_id: UUID
    content_version_id: UUID
    observation_id: UUID
    text: str = Field(min_length=1, max_length=100_000)
    occurred_at: datetime
    metrics: ReportMetricInput

    @field_validator("occurred_at")
    @classmethod
    def require_aware_time(cls, value: datetime) -> datetime:
        if value.utcoffset() is None:
            raise ValueError("comment occurrence time must be timezone-aware")
        return value


class ReportSourceCoverage(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    source_key: str = Field(min_length=1, max_length=64)
    status: SourceCoverageStatus
    succeeded_jobs: int = Field(ge=0)
    partial_jobs: int = Field(ge=0)
    failed_jobs: int = Field(ge=0)
    incomplete_jobs: int = Field(ge=0)


class ReportBuildDataset(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    posts: tuple[ReportPostInput, ...]
    comments: tuple[ReportCommentInput, ...]
    source_coverage: tuple[ReportSourceCoverage, ...]


class ReportInputManifest(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    content_version_ids: tuple[UUID, ...]
    annotation_ids: tuple[UUID, ...]
    observation_ids: tuple[UUID, ...]
    comment_content_version_ids: tuple[UUID, ...]
    source_coverage: tuple[ReportSourceCoverage, ...]
    discovered_at_count: int = Field(ge=0)
    unanalyzed_count: int = Field(ge=0)


class ReportComparison(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    current: int = Field(ge=0)
    previous: int = Field(ge=0)
    delta: int


class ReportOverview(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    posts: ReportComparison
    comments: ReportComparison
    platform_distribution: dict[str, int]
    sentiment_distribution: dict[ReportSentiment, int]


class ReportRepresentativeComment(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    content_id: UUID
    content_version_id: UUID
    text: str
    interaction_count: int = Field(ge=0)


class ReportContentItem(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    citation: str = Field(pattern=r"^c[1-9][0-9]*$")
    content_id: UUID
    content_version_id: UUID
    title: str
    summary: str
    sentiment: ReportSentiment
    source_key: str
    url: str | None
    interaction_count: int = Field(ge=0)
    representative_comments: tuple[ReportRepresentativeComment, ...]


class ReportRiskItem(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    citation: str
    title: str
    url: str | None
    reason: str
    interaction_count: int = Field(ge=0)


class ReportVoiceItem(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    citation: str
    excerpt: str
    url: str | None


class ReportCoverage(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    sources: tuple[ReportSourceCoverage, ...]
    discovered_at_count: int = Field(ge=0)
    unanalyzed_count: int = Field(ge=0)
    disclaimer: str = "样本观察，不代表全网"  # noqa: RUF001


class DailyReportData(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    topic_id: UUID
    topic_name: str
    window_start: datetime
    window_end: datetime
    cutoff_at: datetime
    overview: ReportOverview
    top_contents: tuple[ReportContentItem, ...]
    risks: tuple[ReportRiskItem, ...]
    voices: tuple[ReportVoiceItem, ...]
    coverage: ReportCoverage

    @field_validator("window_start", "window_end", "cutoff_at")
    @classmethod
    def require_utc(cls, value: datetime) -> datetime:
        if value.utcoffset() != timedelta(0):
            raise ValueError("report persistence times must be UTC")
        return value

    @model_validator(mode="after")
    def validate_window(self) -> Self:
        if not self.window_start < self.window_end <= self.cutoff_at:
            raise ValueError("report window and cutoff must be ordered")
        return self


class PreparedDailyReport(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    owner_id: UUID
    topic_id: UUID
    version: int = Field(ge=1)
    window_start: datetime
    window_end: datetime
    cutoff_at: datetime
    input_manifest: ReportInputManifest
    data: DailyReportData
    body_markdown: str = Field(min_length=1)


class DailyReportJobScope(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    topic_id: UUID
    window_start: datetime
    window_end: datetime

    @field_validator("window_start", "window_end")
    @classmethod
    def require_utc(cls, value: datetime) -> datetime:
        if value.utcoffset() != timedelta(0):
            raise ValueError("daily report job windows must be UTC")
        return value

    @model_validator(mode="after")
    def validate_window(self) -> Self:
        if self.window_end - self.window_start != timedelta(days=1):
            raise ValueError("daily report job window must be one day")
        return self

    @classmethod
    def from_job_scope(cls, scope: dict[str, str | int | bool | None]) -> DailyReportJobScope:
        return cls.model_validate(
            {
                "topic_id": scope.get("topic_id"),
                "window_start": scope.get("window_start"),
                "window_end": scope.get("window_end"),
            }
        )


class ReportView(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, from_attributes=True)

    id: UUID
    owner_id: UUID
    topic_id: UUID
    kind: ReportKind
    window_start: datetime
    window_end: datetime
    cutoff_at: datetime
    version: int
    status: ReportStatus
    generator: ReportGenerator
    input_manifest: ReportInputManifest
    data: DailyReportData
    body_markdown: str
    created_at: datetime
