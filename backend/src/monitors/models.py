from datetime import datetime
from uuid import UUID

from sqlalchemy import CheckConstraint, DateTime, ForeignKey, Integer, String, UniqueConstraint
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class Monitor(Base):
    __tablename__ = "monitors"
    __table_args__ = (
        CheckConstraint("state IN ('draft', 'active', 'paused')", name="ck_monitors_state"),
        CheckConstraint("current_version > 0", name="ck_monitors_current_version"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    state: Mapped[str] = mapped_column(String(16))
    current_version: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class MonitorVersion(Base):
    __tablename__ = "monitor_versions"
    __table_args__ = (
        UniqueConstraint("monitor_id", "version", name="uq_monitor_versions_monitor_version"),
        CheckConstraint("version > 0", name="ck_monitor_versions_version"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    monitor_id: Mapped[UUID] = mapped_column(
        ForeignKey("monitors.id", ondelete="CASCADE"), index=True
    )
    version: Mapped[int] = mapped_column(Integer)
    title: Mapped[str] = mapped_column(String(100))
    query_spec: Mapped[dict[str, list[str]]] = mapped_column(JSONB)
    source_ids: Mapped[list[str]] = mapped_column(JSONB)
    schedule: Mapped[dict[str, int]] = mapped_column(JSONB)
    budget: Mapped[dict[str, int]] = mapped_column(JSONB)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class MonitorMatch(Base):
    __tablename__ = "monitor_matches"
    __table_args__ = (
        UniqueConstraint(
            "monitor_version_id", "content_id", name="uq_monitor_matches_version_content"
        ),
        CheckConstraint(
            "relevance_status IN ('pending', 'accepted', 'rejected', 'needs_review')",
            name="ck_monitor_matches_relevance",
        ),
        CheckConstraint(
            "review_state IN ('new', 'ignored', 'following')",
            name="ck_monitor_matches_review_state",
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    monitor_version_id: Mapped[UUID] = mapped_column(
        ForeignKey("monitor_versions.id", ondelete="CASCADE"), index=True
    )
    content_id: Mapped[UUID] = mapped_column(
        ForeignKey("contents.id", ondelete="CASCADE"), index=True
    )
    match_reason: Mapped[list[str]] = mapped_column(JSONB)
    relevance_status: Mapped[str] = mapped_column(String(20))
    review_state: Mapped[str] = mapped_column(String(16))
    first_seen_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    last_seen_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
