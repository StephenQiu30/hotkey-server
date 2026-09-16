from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    DateTime,
    ForeignKey,
    Integer,
    String,
    UniqueConstraint,
)
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class Notification(Base):
    __tablename__ = "notifications"
    __table_args__ = (
        UniqueConstraint(
            "rule_version", "event_id", "change_id", name="uq_notifications_event_change"
        ),
        UniqueConstraint("trend_occurrence_id", name="uq_notifications_trend_occurrence"),
        CheckConstraint("rule_version > 0", name="ck_notifications_rule_version"),
        CheckConstraint(
            "kind IN ('event_member_added', 'event_member_removed', "
            "'event_merged_in', 'event_merged_out', 'event_split_in', 'event_split_out', "
            "'trend_threshold_reached')",
            name="ck_notifications_kind",
        ),
        CheckConstraint(
            "(change_id IS NULL) <> (trend_occurrence_id IS NULL)",
            name="ck_notifications_exactly_one_subject",
        ),
        CheckConstraint(
            "read_at IS NULL OR read_at >= created_at", name="ck_notifications_read_time"
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    event_id: Mapped[UUID] = mapped_column(ForeignKey("events.id", ondelete="CASCADE"), index=True)
    change_id: Mapped[UUID | None] = mapped_column(
        ForeignKey("event_revisions.id", ondelete="CASCADE"), index=True
    )
    trend_occurrence_id: Mapped[UUID | None] = mapped_column(
        ForeignKey("trend_alert_occurrences.id", ondelete="CASCADE"), index=True
    )
    rule_version: Mapped[int] = mapped_column(Integer)
    kind: Mapped[str] = mapped_column(String(32))
    message: Mapped[str] = mapped_column(String(300))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), index=True)
    read_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))


class TrendAlertRule(Base):
    __tablename__ = "trend_alert_rules"
    __table_args__ = (
        UniqueConstraint(
            "event_id",
            "source",
            "metric",
            "bucket_hours",
            name="uq_trend_alert_rules_scope",
        ),
        CheckConstraint(
            "metric IN ('new_posts', 'new_discussions', 'observed_reply_delta')",
            name="ck_trend_alert_rules_metric",
        ),
        CheckConstraint("bucket_hours IN (1, 6, 24)", name="ck_trend_alert_rules_bucket"),
        CheckConstraint("threshold_count > 0", name="ck_trend_alert_rules_threshold"),
        CheckConstraint("version > 0", name="ck_trend_alert_rules_version"),
        CheckConstraint("updated_at >= created_at", name="ck_trend_alert_rules_time"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    event_id: Mapped[UUID] = mapped_column(ForeignKey("events.id", ondelete="CASCADE"), index=True)
    source: Mapped[str] = mapped_column(String(32))
    metric: Mapped[str] = mapped_column(String(32))
    bucket_hours: Mapped[int] = mapped_column(Integer)
    threshold_count: Mapped[int] = mapped_column(Integer)
    version: Mapped[int] = mapped_column(Integer)
    enabled: Mapped[bool] = mapped_column(Boolean)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class TrendAlertOccurrence(Base):
    __tablename__ = "trend_alert_occurrences"
    __table_args__ = (
        UniqueConstraint(
            "rule_id",
            "rule_version",
            "bucket_start",
            name="uq_trend_alert_occurrences_bucket",
        ),
        CheckConstraint("rule_version > 0", name="ck_trend_alert_occurrences_version"),
        CheckConstraint("metric_value >= 0", name="ck_trend_alert_occurrences_value"),
        CheckConstraint("bucket_end > bucket_start", name="ck_trend_alert_occurrences_window"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    rule_id: Mapped[UUID] = mapped_column(
        ForeignKey("trend_alert_rules.id", ondelete="CASCADE"), index=True
    )
    event_id: Mapped[UUID] = mapped_column(ForeignKey("events.id", ondelete="CASCADE"), index=True)
    rule_version: Mapped[int] = mapped_column(Integer)
    bucket_start: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    bucket_end: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    metric_value: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), index=True)
