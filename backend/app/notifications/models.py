from __future__ import annotations

from datetime import datetime
from typing import Any
from uuid import UUID

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    ForeignKey,
    ForeignKeyConstraint,
    Integer,
    String,
    UniqueConstraint,
    text,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class NotificationTarget(Base):
    __tablename__ = "notification_targets"
    __table_args__ = (
        UniqueConstraint("owner_id", "name", name="notification_targets_owner_name_key"),
        UniqueConstraint("owner_id", "id", name="notification_targets_owner_id_key"),
        CheckConstraint(
            "channel IN ('feishu', 'email')", name="notification_targets_channel_check"
        ),
        CheckConstraint(
            "jsonb_typeof(recipients) = 'array'", name="notification_targets_recipients_check"
        ),
        CheckConstraint(
            "secret_env IS NULL OR secret_env ~ '^HOTKEY_[A-Z][A-Z0-9_]*$'",
            name="notification_targets_secret_env_check",
        ),
        CheckConstraint("updated_at >= created_at", name="notification_targets_updated_at_check"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    name: Mapped[str] = mapped_column(String(80))
    channel: Mapped[str] = mapped_column(String(16))
    recipients: Mapped[list[Any]] = mapped_column(JSONB)
    secret_env: Mapped[str | None] = mapped_column(String(128))
    enabled: Mapped[bool] = mapped_column(Boolean, server_default=text("true"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class NotificationDelivery(Base):
    __tablename__ = "notification_deliveries"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "target_id"],
            ["notification_targets.owner_id", "notification_targets.id"],
            ondelete="CASCADE",
            name="notification_deliveries_owner_target_fkey",
        ),
        UniqueConstraint(
            "report_id",
            "report_version",
            "target_id",
            name="notification_deliveries_report_version_target_key",
        ),
        CheckConstraint("report_version >= 1", name="notification_deliveries_version_check"),
        CheckConstraint(
            "status IN ('pending', 'sending', 'succeeded', 'failed', 'unknown')",
            name="notification_deliveries_status_check",
        ),
        CheckConstraint(
            "attempt_count BETWEEN 0 AND 3", name="notification_deliveries_attempt_check"
        ),
        CheckConstraint(
            "updated_at >= created_at", name="notification_deliveries_updated_at_check"
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    report_id: Mapped[UUID] = mapped_column(ForeignKey("reports.id", ondelete="CASCADE"))
    report_version: Mapped[int] = mapped_column(Integer)
    target_id: Mapped[UUID]
    status: Mapped[str] = mapped_column(String(16))
    attempt_count: Mapped[int] = mapped_column(Integer, server_default=text("0"))
    last_error_code: Mapped[str | None] = mapped_column(String(64))
    sent_at: Mapped[datetime | None]
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]
