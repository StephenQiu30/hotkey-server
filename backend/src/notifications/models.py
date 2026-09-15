from datetime import datetime
from uuid import UUID

from sqlalchemy import CheckConstraint, DateTime, ForeignKey, Integer, String, UniqueConstraint
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class Notification(Base):
    __tablename__ = "notifications"
    __table_args__ = (
        UniqueConstraint(
            "rule_version", "event_id", "change_id", name="uq_notifications_event_change"
        ),
        CheckConstraint("rule_version > 0", name="ck_notifications_rule_version"),
        CheckConstraint(
            "kind IN ('event_member_added', 'event_member_removed', "
            "'event_merged_in', 'event_merged_out', 'event_split_in', 'event_split_out')",
            name="ck_notifications_kind",
        ),
        CheckConstraint(
            "read_at IS NULL OR read_at >= created_at", name="ck_notifications_read_time"
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    event_id: Mapped[UUID] = mapped_column(ForeignKey("events.id", ondelete="CASCADE"), index=True)
    change_id: Mapped[UUID] = mapped_column(
        ForeignKey("event_revisions.id", ondelete="CASCADE"), index=True
    )
    rule_version: Mapped[int] = mapped_column(Integer)
    kind: Mapped[str] = mapped_column(String(32))
    message: Mapped[str] = mapped_column(String(300))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), index=True)
    read_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
