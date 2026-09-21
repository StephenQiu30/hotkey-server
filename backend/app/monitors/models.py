from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import CheckConstraint, ForeignKey, Index, Integer, String, text
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class MonitorTopic(Base):
    __tablename__ = "monitor_topics"
    __table_args__ = (
        CheckConstraint(
            "char_length(name) BETWEEN 1 AND 80",
            name="monitor_topics_name_length_check",
        ),
        CheckConstraint(
            "status IN ('paused', 'active', 'archived')",
            name="monitor_topics_status_check",
        ),
        CheckConstraint(
            "readiness_status IN ('pending_source_selection', 'pending_source_readiness', 'ready')",
            name="monitor_topics_readiness_status_check",
        ),
        CheckConstraint(
            "current_version >= 1",
            name="monitor_topics_current_version_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="monitor_topics_updated_at_check",
        ),
        Index("monitor_topics_owner_updated_idx", "owner_id", "updated_at"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    name: Mapped[str] = mapped_column(String(80))
    status: Mapped[str] = mapped_column(String(16), server_default=text("'paused'"))
    readiness_status: Mapped[str] = mapped_column(
        String(32),
        server_default=text("'pending_source_selection'"),
    )
    current_version: Mapped[int] = mapped_column(Integer, server_default=text("1"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class MonitorTopicVersion(Base):
    __tablename__ = "monitor_topic_versions"
    __table_args__ = (
        CheckConstraint("version >= 1", name="monitor_topic_versions_version_check"),
        CheckConstraint(
            "jsonb_typeof(match_any) = 'array'",
            name="monitor_topic_versions_match_any_check",
        ),
        CheckConstraint(
            "jsonb_typeof(match_all) = 'array'",
            name="monitor_topic_versions_match_all_check",
        ),
        CheckConstraint(
            "jsonb_typeof(exclude) = 'array'",
            name="monitor_topic_versions_exclude_check",
        ),
        Index("monitor_topic_versions_created_by_idx", "created_by"),
    )

    topic_id: Mapped[UUID] = mapped_column(
        ForeignKey("monitor_topics.id", ondelete="CASCADE"),
        primary_key=True,
    )
    version: Mapped[int] = mapped_column(Integer, primary_key=True)
    created_by: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="RESTRICT"))
    match_any: Mapped[list[str]] = mapped_column(JSONB)
    match_all: Mapped[list[str]] = mapped_column(JSONB)
    exclude: Mapped[list[str]] = mapped_column(JSONB)
    created_at: Mapped[datetime]
