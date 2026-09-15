from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    CheckConstraint,
    DateTime,
    ForeignKey,
    Integer,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class Event(Base):
    __tablename__ = "events"
    __table_args__ = (
        CheckConstraint("status IN ('active', 'archived')", name="ck_events_status"),
        CheckConstraint("current_revision > 0", name="ck_events_current_revision"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    title: Mapped[str] = mapped_column(String(200))
    summary: Mapped[str] = mapped_column(Text)
    status: Mapped[str] = mapped_column(String(16))
    current_revision: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class EventMember(Base):
    __tablename__ = "event_members"
    __table_args__ = (UniqueConstraint("content_id", name="uq_event_members_content"),)
    event_id: Mapped[UUID] = mapped_column(
        ForeignKey("events.id", ondelete="CASCADE"), primary_key=True
    )
    content_id: Mapped[UUID] = mapped_column(
        ForeignKey("contents.id", ondelete="RESTRICT"), primary_key=True
    )
    added_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class EventRevision(Base):
    __tablename__ = "event_revisions"
    __table_args__ = (
        UniqueConstraint("event_id", "revision", name="uq_event_revisions_number"),
        CheckConstraint("revision > 0", name="ck_event_revisions_revision"),
        CheckConstraint(
            "change_type IN ('create', 'add_member', 'remove_member', "
            "'merge_in', 'merge_out', 'split_in', 'split_out')",
            name="ck_event_revisions_change_type",
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    event_id: Mapped[UUID] = mapped_column(ForeignKey("events.id", ondelete="CASCADE"), index=True)
    revision: Mapped[int] = mapped_column(Integer)
    change_type: Mapped[str] = mapped_column(String(32))
    related_event_id: Mapped[UUID | None] = mapped_column(ForeignKey("events.id"))
    snapshot: Mapped[dict[str, object]] = mapped_column(JSONB)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
