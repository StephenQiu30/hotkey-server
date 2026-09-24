from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    CheckConstraint,
    ForeignKey,
    ForeignKeyConstraint,
    Index,
    Integer,
    String,
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
