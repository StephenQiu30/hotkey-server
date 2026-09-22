from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    BigInteger,
    CheckConstraint,
    ForeignKey,
    ForeignKeyConstraint,
    Index,
    String,
    UniqueConstraint,
)
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class ContentRecord(Base):
    __tablename__ = "content_records"
    __table_args__ = (
        UniqueConstraint("owner_id", "id", name="content_records_owner_id_key"),
        UniqueConstraint(
            "owner_id",
            "source_key",
            "object_type",
            "native_scope",
            "external_id",
            name="content_records_source_identity_key",
            postgresql_nulls_not_distinct=True,
        ),
        CheckConstraint(
            "source_key ~ '^[a-z][a-z0-9_-]{0,63}$'",
            name="content_records_source_key_check",
        ),
        CheckConstraint(
            "object_type IN ('post', 'comment')",
            name="content_records_object_type_check",
        ),
        CheckConstraint(
            "native_scope IS NULL OR native_scope <> ''",
            name="content_records_native_scope_check",
        ),
        CheckConstraint("external_id <> ''", name="content_records_external_id_check"),
        Index("content_records_owner_id_idx", "owner_id", "id"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    source_key: Mapped[str] = mapped_column(String(64))
    object_type: Mapped[str] = mapped_column(String(16))
    native_scope: Mapped[str | None] = mapped_column(String(512))
    external_id: Mapped[str] = mapped_column(String(512))
    created_at: Mapped[datetime]


class ContentDiscovery(Base):
    __tablename__ = "content_discoveries"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "content_id"],
            ["content_records.owner_id", "content_records.id"],
            ondelete="CASCADE",
            name="content_discoveries_owner_content_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "job_id"],
            ["jobs.owner_id", "jobs.id"],
            ondelete="RESTRICT",
            name="content_discoveries_owner_job_fkey",
        ),
        UniqueConstraint(
            "owner_id",
            "content_id",
            "job_id",
            name="content_discoveries_owner_content_job_key",
        ),
        Index("content_discoveries_content_idx", "owner_id", "content_id"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    content_id: Mapped[UUID]
    job_id: Mapped[UUID]
    first_observed_at: Mapped[datetime]
    created_at: Mapped[datetime]


class ContentObservation(Base):
    __tablename__ = "content_observations"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "content_id"],
            ["content_records.owner_id", "content_records.id"],
            ondelete="CASCADE",
            name="content_observations_owner_content_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "job_id"],
            ["jobs.owner_id", "jobs.id"],
            ondelete="RESTRICT",
            name="content_observations_owner_job_fkey",
        ),
        UniqueConstraint(
            "owner_id",
            "content_id",
            "source_operation_id",
            name="content_observations_owner_content_operation_key",
        ),
        CheckConstraint(
            "received_at >= observed_at",
            name="content_observations_received_at_check",
        ),
        CheckConstraint(
            "canonical_url IS NULL OR canonical_url ~ '^https?://'",
            name="content_observations_canonical_url_check",
        ),
        CheckConstraint(
            "author_external_id IS NULL OR author_external_id <> ''",
            name="content_observations_author_check",
        ),
        CheckConstraint(
            "(like_count IS NULL OR like_count >= 0) AND "
            "(comment_count IS NULL OR comment_count >= 0) AND "
            "(repost_count IS NULL OR repost_count >= 0) AND "
            "(view_count IS NULL OR view_count >= 0) AND "
            "(play_count IS NULL OR play_count >= 0) AND "
            "(danmaku_count IS NULL OR danmaku_count >= 0)",
            name="content_observations_metrics_check",
        ),
        Index(
            "content_observations_latest_idx",
            "owner_id",
            "content_id",
            "observed_at",
            "received_at",
            "id",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    content_id: Mapped[UUID]
    job_id: Mapped[UUID]
    source_operation_id: Mapped[UUID]
    observed_at: Mapped[datetime]
    received_at: Mapped[datetime]
    published_at: Mapped[datetime | None]
    canonical_url: Mapped[str | None] = mapped_column(String(2048))
    author_external_id: Mapped[str | None] = mapped_column(String(512))
    like_count: Mapped[int | None] = mapped_column(BigInteger)
    comment_count: Mapped[int | None] = mapped_column(BigInteger)
    repost_count: Mapped[int | None] = mapped_column(BigInteger)
    view_count: Mapped[int | None] = mapped_column(BigInteger)
    play_count: Mapped[int | None] = mapped_column(BigInteger)
    danmaku_count: Mapped[int | None] = mapped_column(BigInteger)
