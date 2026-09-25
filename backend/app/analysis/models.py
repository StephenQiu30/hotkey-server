from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    ForeignKeyConstraint,
    Index,
    String,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class ContentAnnotation(Base):
    __tablename__ = "content_annotations"
    __table_args__ = (
        UniqueConstraint("owner_id", "id", name="content_annotations_owner_id_key"),
        UniqueConstraint(
            "owner_id",
            "content_version_id",
            "topic_id",
            "topic_rule_version",
            "prompt_version",
            name="content_annotations_owner_version_topic_rule_prompt_key",
        ),
        ForeignKeyConstraint(
            ["owner_id", "content_id", "content_version_id"],
            ["content_versions.owner_id", "content_versions.content_id", "content_versions.id"],
            ondelete="CASCADE",
            name="content_annotations_owner_content_version_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "topic_id"],
            ["monitor_topics.owner_id", "monitor_topics.id"],
            ondelete="CASCADE",
            name="content_annotations_owner_topic_fkey",
        ),
        ForeignKeyConstraint(
            ["topic_id", "topic_rule_version"],
            ["monitor_topic_versions.topic_id", "monitor_topic_versions.version"],
            ondelete="CASCADE",
            name="content_annotations_topic_rule_version_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "ai_call_id"],
            ["ai_calls.owner_id", "ai_calls.id"],
            name="content_annotations_owner_ai_call_fkey",
        ),
        CheckConstraint("topic_rule_version >= 1", name="content_annotations_rule_version_check"),
        CheckConstraint("prompt_version <> ''", name="content_annotations_prompt_version_check"),
        CheckConstraint(
            "sentiment IS NULL OR sentiment IN ('positive', 'neutral', 'negative')",
            name="content_annotations_sentiment_check",
        ),
        CheckConstraint(
            "jsonb_typeof(viewpoints) = 'array' AND jsonb_array_length(viewpoints) <= 5",
            name="content_annotations_viewpoints_check",
        ),
        CheckConstraint(
            "status IN ('annotated', 'unanalyzed')",
            name="content_annotations_status_check",
        ),
        CheckConstraint(
            "(status = 'annotated' AND relevant IS NOT NULL "
            "AND relevance_reason IS NOT NULL AND summary IS NOT NULL "
            "AND ((relevant AND sentiment IS NOT NULL) OR (NOT relevant AND sentiment IS NULL))) "
            "OR (status = 'unanalyzed' AND relevant IS NULL AND relevance_reason IS NULL "
            "AND sentiment IS NULL AND summary IS NULL AND viewpoints = '[]'::jsonb)",
            name="content_annotations_output_status_check",
        ),
        Index(
            "content_annotations_topic_created_idx",
            "owner_id",
            "topic_id",
            "created_at",
        ),
        Index(
            "content_annotations_content_idx",
            "owner_id",
            "content_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    content_id: Mapped[UUID]
    content_version_id: Mapped[UUID]
    topic_id: Mapped[UUID]
    topic_rule_version: Mapped[int]
    prompt_version: Mapped[str] = mapped_column(String(128))
    relevant: Mapped[bool | None] = mapped_column(Boolean)
    relevance_reason: Mapped[str | None] = mapped_column(String(500))
    sentiment: Mapped[str | None] = mapped_column(String(16))
    summary: Mapped[str | None] = mapped_column(String(60))
    viewpoints: Mapped[list[str]] = mapped_column(JSONB)
    ai_call_id: Mapped[UUID | None]
    status: Mapped[str] = mapped_column(String(16))
    created_at: Mapped[datetime]
