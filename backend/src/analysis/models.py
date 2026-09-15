from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class AnalysisRun(Base):
    __tablename__ = "analysis_runs"
    __table_args__ = (
        UniqueConstraint("event_id", "request_sha256", name="uq_analysis_runs_request"),
        CheckConstraint("event_revision > 0", name="ck_analysis_runs_event_revision"),
        CheckConstraint("since_at < until_at", name="ck_analysis_runs_window"),
        CheckConstraint("until_at <= cutoff_at", name="ck_analysis_runs_cutoff"),
        CheckConstraint("max_items BETWEEN 1 AND 100", name="ck_analysis_runs_max_items"),
        CheckConstraint(
            "sample_count BETWEEN 1 AND max_items", name="ck_analysis_runs_sample_count"
        ),
        CheckConstraint("state IN ('pending', 'succeeded')", name="ck_analysis_runs_state"),
        CheckConstraint(
            "token_budget = 0 AND input_tokens = 0 AND output_tokens = 0",
            name="ck_analysis_runs_manual_tokens",
        ),
        Index("ix_analysis_runs_event_created", "event_id", "created_at"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    event_id: Mapped[UUID] = mapped_column(ForeignKey("events.id", ondelete="CASCADE"))
    event_revision: Mapped[int] = mapped_column(Integer)
    state: Mapped[str] = mapped_column(String(16))
    method: Mapped[str] = mapped_column(String(32))
    analyzer_id: Mapped[str] = mapped_column(String(64))
    prompt_version: Mapped[str] = mapped_column(String(64))
    label_schema_version: Mapped[str] = mapped_column(String(64))
    sampling_policy_version: Mapped[str] = mapped_column(String(64))
    request_sha256: Mapped[str] = mapped_column(String(64))
    manifest_sha256: Mapped[str] = mapped_column(String(64))
    since_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    until_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    cutoff_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    max_items: Mapped[int] = mapped_column(Integer)
    sample_count: Mapped[int] = mapped_column(Integer)
    token_budget: Mapped[int] = mapped_column(Integer)
    input_tokens: Mapped[int] = mapped_column(Integer)
    output_tokens: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class AnalysisSample(Base):
    __tablename__ = "analysis_samples"
    __table_args__ = (
        UniqueConstraint("run_id", "position", name="uq_analysis_samples_position"),
        UniqueConstraint("run_id", "content_version_id", name="uq_analysis_samples_version"),
        CheckConstraint("position > 0", name="ck_analysis_samples_position"),
        CheckConstraint("kind IN ('comment', 'reply')", name="ck_analysis_samples_kind"),
        CheckConstraint(
            "ordering_origin IN ('provider_default')",
            name="ck_analysis_samples_ordering_origin",
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    run_id: Mapped[UUID] = mapped_column(
        ForeignKey("analysis_runs.id", ondelete="CASCADE"), index=True
    )
    position: Mapped[int] = mapped_column(Integer)
    content_id: Mapped[UUID] = mapped_column(ForeignKey("contents.id", ondelete="RESTRICT"))
    content_version_id: Mapped[UUID] = mapped_column(
        ForeignKey("content_versions.id", ondelete="RESTRICT")
    )
    parent_content_version_id: Mapped[UUID | None] = mapped_column(
        ForeignKey("content_versions.id", ondelete="RESTRICT")
    )
    root_content_version_id: Mapped[UUID | None] = mapped_column(
        ForeignKey("content_versions.id", ondelete="RESTRICT")
    )
    source: Mapped[str] = mapped_column(String(32))
    kind: Mapped[str] = mapped_column(String(16))
    root_external_id: Mapped[str] = mapped_column(String(1024))
    published_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    time_bucket_start: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    ordering_origin: Mapped[str] = mapped_column(String(32))
    selection_reason: Mapped[str] = mapped_column(String(64))
    text_sha256: Mapped[str] = mapped_column(String(64))
    parent_text_sha256: Mapped[str | None] = mapped_column(String(64))
    root_text_sha256: Mapped[str | None] = mapped_column(String(64))


class AnalysisLabel(Base):
    __tablename__ = "analysis_labels"
    __table_args__ = (
        UniqueConstraint("sample_id", name="uq_analysis_labels_sample"),
        CheckConstraint(
            "sentiment IN ('positive', 'negative', 'neutral', 'mixed', 'unknown')",
            name="ck_analysis_labels_sentiment",
        ),
        CheckConstraint(
            "stance IN ('support', 'oppose', 'neutral', 'mixed', 'unknown')",
            name="ck_analysis_labels_stance",
        ),
        CheckConstraint("label_source IN ('manual')", name="ck_analysis_labels_source"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    sample_id: Mapped[UUID] = mapped_column(
        ForeignKey("analysis_samples.id", ondelete="CASCADE"), index=True
    )
    topic: Mapped[str] = mapped_column(String(80))
    target: Mapped[str] = mapped_column(String(160))
    sentiment: Mapped[str] = mapped_column(String(16))
    stance: Mapped[str] = mapped_column(String(16))
    request: Mapped[str] = mapped_column(Text)
    abstained: Mapped[bool] = mapped_column(Boolean)
    citation_content_version_ids: Mapped[list[str]] = mapped_column(JSONB)
    label_source: Mapped[str] = mapped_column(String(16))
    schema_version: Mapped[str] = mapped_column(String(64))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
