from __future__ import annotations

from datetime import datetime
from typing import Any
from uuid import UUID

from sqlalchemy import (
    CheckConstraint,
    ForeignKeyConstraint,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class Report(Base):
    __tablename__ = "reports"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "topic_id"],
            ["monitor_topics.owner_id", "monitor_topics.id"],
            ondelete="CASCADE",
            name="reports_owner_topic_fkey",
        ),
        UniqueConstraint(
            "owner_id",
            "topic_id",
            "kind",
            "window_start",
            "version",
            name="reports_owner_topic_kind_window_version_key",
        ),
        CheckConstraint("kind IN ('daily', 'weekly')", name="reports_kind_check"),
        CheckConstraint("window_start < window_end", name="reports_window_check"),
        CheckConstraint("cutoff_at >= window_end", name="reports_cutoff_check"),
        CheckConstraint("version >= 1", name="reports_version_check"),
        CheckConstraint("status IN ('draft', 'final')", name="reports_status_check"),
        CheckConstraint("generator IN ('template', 'model')", name="reports_generator_check"),
        CheckConstraint(
            "jsonb_typeof(input_manifest) = 'object'",
            name="reports_input_manifest_object_check",
        ),
        CheckConstraint("jsonb_typeof(data) = 'object'", name="reports_data_object_check"),
        CheckConstraint("body_markdown <> ''", name="reports_body_markdown_check"),
        CheckConstraint("created_at >= cutoff_at", name="reports_created_cutoff_check"),
        Index(
            "reports_owner_topic_window_idx",
            "owner_id",
            "topic_id",
            "kind",
            "window_start",
            "status",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    topic_id: Mapped[UUID]
    kind: Mapped[str] = mapped_column(String(16))
    window_start: Mapped[datetime]
    window_end: Mapped[datetime]
    cutoff_at: Mapped[datetime]
    version: Mapped[int] = mapped_column(Integer)
    status: Mapped[str] = mapped_column(String(16))
    generator: Mapped[str] = mapped_column(String(16))
    input_manifest: Mapped[dict[str, Any]] = mapped_column(JSONB)
    data: Mapped[dict[str, Any]] = mapped_column(JSONB)
    body_markdown: Mapped[str] = mapped_column(Text)
    created_at: Mapped[datetime]
