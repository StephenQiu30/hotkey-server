from __future__ import annotations

from datetime import datetime, time
from uuid import UUID

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    ForeignKey,
    ForeignKeyConstraint,
    Index,
    Integer,
    PrimaryKeyConstraint,
    String,
    Time,
    UniqueConstraint,
    text,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from db.base import Base


class FollowedAccount(Base):
    __tablename__ = "followed_accounts"
    __table_args__ = (
        CheckConstraint(
            "length(source_key) BETWEEN 1 AND 64",
            name="followed_accounts_source_key_length_check",
        ),
        CheckConstraint(
            "length(external_id) BETWEEN 1 AND 256",
            name="followed_accounts_external_id_length_check",
        ),
        CheckConstraint(
            "display_name IS NULL OR length(display_name) BETWEEN 1 AND 256",
            name="followed_accounts_display_name_length_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="followed_accounts_updated_at_check",
        ),
        UniqueConstraint(
            "owner_id",
            "source_key",
            "external_id",
            name="followed_accounts_owner_source_identity_key",
        ),
        UniqueConstraint("owner_id", "id", name="followed_accounts_owner_id_key"),
        Index("followed_accounts_owner_created_idx", "owner_id", "created_at", "id"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    source_key: Mapped[str] = mapped_column(String(64))
    external_id: Mapped[str] = mapped_column(String(256))
    display_name: Mapped[str | None] = mapped_column(String(256))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]

    aliases: Mapped[list[FollowedAccountAlias]] = relationship(
        back_populates="account",
        cascade="all, delete-orphan",
        passive_deletes=True,
    )


class FollowedAccountAlias(Base):
    __tablename__ = "followed_account_aliases"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "account_id"],
            ["followed_accounts.owner_id", "followed_accounts.id"],
            ondelete="CASCADE",
            name="followed_account_aliases_owner_account_fkey",
        ),
        CheckConstraint(
            "length(alias_value) BETWEEN 1 AND 128",
            name="followed_account_aliases_value_length_check",
        ),
        CheckConstraint(
            "last_seen_at >= first_seen_at",
            name="followed_account_aliases_seen_at_check",
        ),
        Index("followed_account_aliases_owner_value_idx", "owner_id", "alias_value"),
    )

    owner_id: Mapped[UUID] = mapped_column(primary_key=True)
    account_id: Mapped[UUID] = mapped_column(primary_key=True)
    alias_value: Mapped[str] = mapped_column(String(128), primary_key=True)
    first_seen_at: Mapped[datetime]
    last_seen_at: Mapped[datetime]

    account: Mapped[FollowedAccount] = relationship(back_populates="aliases")


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
            "collection_interval_seconds BETWEEN 600 AND 86400",
            name="monitor_topics_collection_interval_check",
        ),
        CheckConstraint(
            "report_timezone = 'Asia/Shanghai'",
            name="monitor_topics_report_timezone_check",
        ),
        CheckConstraint(
            "jsonb_typeof(notification_target_names) = 'array'",
            name="monitor_topics_notification_targets_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="monitor_topics_updated_at_check",
        ),
        UniqueConstraint("owner_id", "id", name="monitor_topics_owner_id_key"),
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
    collection_interval_seconds: Mapped[int] = mapped_column(
        Integer,
        server_default=text("1800"),
    )
    report_time: Mapped[time] = mapped_column(
        Time(timezone=False),
        server_default=text("'09:00:00'"),
    )
    report_timezone: Mapped[str] = mapped_column(
        String(64),
        server_default=text("'Asia/Shanghai'"),
    )
    weekly_report_enabled: Mapped[bool] = mapped_column(
        Boolean,
        server_default=text("false"),
    )
    notification_target_names: Mapped[list[str]] = mapped_column(
        JSONB,
        server_default=text("'[]'::jsonb"),
    )
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


class MonitorSchedule(Base):
    __tablename__ = "monitor_schedules"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "topic_id"],
            ["monitor_topics.owner_id", "monitor_topics.id"],
            ondelete="CASCADE",
            name="monitor_schedules_owner_topic_fkey",
        ),
        CheckConstraint(
            "source_key ~ '^[a-z][a-z0-9_-]{0,63}$'",
            name="monitor_schedules_source_key_check",
        ),
        CheckConstraint(
            "capability IN ('search', 'author_posts', 'comments', 'replies', "
            "'page_content', 'hotlist')",
            name="monitor_schedules_capability_check",
        ),
        CheckConstraint(
            "interval_seconds BETWEEN 600 AND 86400",
            name="monitor_schedules_interval_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="monitor_schedules_updated_at_check",
        ),
        PrimaryKeyConstraint(
            "owner_id",
            "topic_id",
            "source_key",
            "capability",
            name="monitor_schedules_owner_topic_source_capability_key",
        ),
        Index("monitor_schedules_enabled_next_run_idx", "enabled", "next_run_at"),
    )

    owner_id: Mapped[UUID]
    topic_id: Mapped[UUID]
    source_key: Mapped[str] = mapped_column(String(64))
    capability: Mapped[str] = mapped_column(String(32))
    interval_seconds: Mapped[int] = mapped_column(Integer)
    next_run_at: Mapped[datetime]
    enabled: Mapped[bool] = mapped_column(Boolean, server_default=text("false"))
    last_job_id: Mapped[UUID | None]
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]
