from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    BigInteger,
    CheckConstraint,
    ForeignKey,
    ForeignKeyConstraint,
    Index,
    LargeBinary,
    SmallInteger,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import JSONB
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
            "object_type IN ('post', 'comment', 'webpage')",
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


class HotlistSnapshot(Base):
    __tablename__ = "hotlist_snapshots"
    __table_args__ = (
        UniqueConstraint("owner_id", "id", name="hotlist_snapshots_owner_id_key"),
        UniqueConstraint("owner_id", "job_id", name="hotlist_snapshots_owner_job_key"),
        ForeignKeyConstraint(
            ["owner_id", "job_id"],
            ["jobs.owner_id", "jobs.id"],
            ondelete="RESTRICT",
            name="hotlist_snapshots_owner_job_fkey",
        ),
        CheckConstraint("source_key ~ '^[a-z][a-z0-9_-]{0,63}$'"),
        CheckConstraint("entry_count BETWEEN 0 AND 100"),
        Index("hotlist_snapshots_latest_idx", "owner_id", "source_key", "observed_at", "id"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    source_key: Mapped[str] = mapped_column(String(64))
    job_id: Mapped[UUID]
    observed_at: Mapped[datetime]
    entry_count: Mapped[int]


class HotlistEntryRecord(Base):
    __tablename__ = "hotlist_entries"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "snapshot_id"],
            ["hotlist_snapshots.owner_id", "hotlist_snapshots.id"],
            ondelete="CASCADE",
            name="hotlist_entries_owner_snapshot_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "content_id"],
            ["content_records.owner_id", "content_records.id"],
            ondelete="SET NULL",
            name="hotlist_entries_owner_content_fkey",
        ),
        CheckConstraint("rank BETWEEN 1 AND 100"),
        CheckConstraint("title <> ''"),
        CheckConstraint("url ~ '^https?://'"),
        CheckConstraint("jsonb_typeof(matched_topic_names) = 'array'"),
        CheckConstraint("jsonb_typeof(matched_topic_ids) = 'array'"),
    )

    snapshot_id: Mapped[UUID] = mapped_column(primary_key=True)
    rank: Mapped[int] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    title: Mapped[str] = mapped_column(String(2000))
    url: Mapped[str] = mapped_column(String(2048))
    summary: Mapped[str | None] = mapped_column(Text)
    heat: Mapped[str | None] = mapped_column(String(256))
    published_at: Mapped[datetime | None]
    content_id: Mapped[UUID | None]
    matched_topic_names: Mapped[list[str]] = mapped_column(JSONB)
    matched_topic_ids: Mapped[list[str]] = mapped_column(JSONB)


class ContentVersion(Base):
    __tablename__ = "content_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "content_id"],
            ["content_records.owner_id", "content_records.id"],
            ondelete="CASCADE",
            name="content_versions_owner_content_fkey",
        ),
        UniqueConstraint("owner_id", "id", name="content_versions_owner_id_key"),
        UniqueConstraint(
            "owner_id",
            "content_id",
            "id",
            name="content_versions_owner_content_id_key",
        ),
        UniqueConstraint(
            "owner_id",
            "content_id",
            "fingerprint",
            name="content_versions_owner_content_fingerprint_key",
        ),
        CheckConstraint(
            "octet_length(fingerprint) = 32",
            name="content_versions_fingerprint_check",
        ),
        CheckConstraint(
            "text_scope IN ('full', 'summary', 'truncated', 'media_only')",
            name="content_versions_text_scope_check",
        ),
        CheckConstraint(
            "text_origin IN ('source', 'machine_extracted')",
            name="content_versions_text_origin_check",
        ),
        CheckConstraint(
            "(text_origin = 'source' AND text_origin_ref IS NULL) OR "
            "(text_origin = 'machine_extracted' AND text_origin_ref IS NOT NULL "
            "AND text_origin_ref <> '')",
            name="content_versions_origin_ref_check",
        ),
        CheckConstraint(
            "(title IS NULL OR (char_length(title) BETWEEN 1 AND 2000)) AND "
            "(body IS NULL OR (char_length(body) BETWEEN 1 AND 100000))",
            name="content_versions_text_length_check",
        ),
        CheckConstraint(
            "(text_scope = 'media_only' AND title IS NULL AND body IS NULL "
            "AND truncation_reason IS NULL AND text_origin = 'source') OR "
            "(text_scope IN ('full', 'summary') AND (title IS NOT NULL OR body IS NOT NULL) "
            "AND truncation_reason IS NULL) OR "
            "(text_scope = 'truncated' AND (title IS NOT NULL OR body IS NOT NULL) "
            "AND truncation_reason IN ('source_limit', 'collector_limit'))",
            name="content_versions_scope_content_check",
        ),
        Index("content_versions_content_idx", "owner_id", "content_id", "created_at", "id"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    content_id: Mapped[UUID]
    fingerprint: Mapped[bytes] = mapped_column(LargeBinary(32))
    text_scope: Mapped[str] = mapped_column(String(16))
    text_origin: Mapped[str] = mapped_column(String(32))
    text_origin_ref: Mapped[str | None] = mapped_column(String(512))
    title: Mapped[str | None] = mapped_column(String(2000))
    body: Mapped[str | None] = mapped_column(Text)
    truncation_reason: Mapped[str | None] = mapped_column(String(32))
    created_at: Mapped[datetime]


class ContentVersionRelation(Base):
    __tablename__ = "content_version_relations"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "content_version_id"],
            ["content_versions.owner_id", "content_versions.id"],
            ondelete="CASCADE",
            name="content_version_relations_owner_version_fkey",
        ),
        UniqueConstraint(
            "owner_id",
            "content_version_id",
            "relation_type",
            "target_native_scope",
            "target_external_id",
            name="content_version_relations_target_key",
            postgresql_nulls_not_distinct=True,
        ),
        CheckConstraint(
            "relation_type IN ('quote', 'repost')",
            name="content_version_relations_type_check",
        ),
        CheckConstraint(
            "target_native_scope IS NULL OR target_native_scope <> ''",
            name="content_version_relations_scope_check",
        ),
        CheckConstraint(
            "target_external_id <> ''",
            name="content_version_relations_external_id_check",
        ),
        CheckConstraint(
            "target_author_external_id IS NULL OR target_author_external_id <> ''",
            name="content_version_relations_author_check",
        ),
        Index(
            "content_version_relations_version_idx",
            "owner_id",
            "content_version_id",
            "relation_type",
            "id",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    content_version_id: Mapped[UUID]
    relation_type: Mapped[str] = mapped_column(String(16))
    target_native_scope: Mapped[str | None] = mapped_column(String(512))
    target_external_id: Mapped[str] = mapped_column(String(512))
    target_author_external_id: Mapped[str | None] = mapped_column(String(512))


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
        ForeignKeyConstraint(
            ["owner_id", "content_id", "content_version_id"],
            ["content_versions.owner_id", "content_versions.content_id", "content_versions.id"],
            ondelete="RESTRICT",
            name="content_observations_owner_content_version_fkey",
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
            "final_url IS NULL OR final_url ~ '^https?://'",
            name="content_observations_final_url_check",
        ),
        CheckConstraint(
            "author_external_id IS NULL OR author_external_id <> ''",
            name="content_observations_author_check",
        ),
        CheckConstraint(
            "author_name IS NULL OR author_name <> ''",
            name="content_observations_author_name_check",
        ),
        CheckConstraint(
            "(published_at IS NULL AND published_at_fractional_digits IS NULL) OR "
            "(published_at IS NOT NULL AND published_at_fractional_digits BETWEEN 0 AND 6)",
            name="content_observations_published_precision_check",
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
    content_version_id: Mapped[UUID | None]
    observed_at: Mapped[datetime]
    received_at: Mapped[datetime]
    published_at: Mapped[datetime | None]
    published_at_fractional_digits: Mapped[int | None] = mapped_column(SmallInteger)
    canonical_url: Mapped[str | None] = mapped_column(String(2048))
    final_url: Mapped[str | None] = mapped_column(String(2048))
    author_external_id: Mapped[str | None] = mapped_column(String(512))
    author_name: Mapped[str | None] = mapped_column(String(256))
    like_count: Mapped[int | None] = mapped_column(BigInteger)
    comment_count: Mapped[int | None] = mapped_column(BigInteger)
    repost_count: Mapped[int | None] = mapped_column(BigInteger)
    view_count: Mapped[int | None] = mapped_column(BigInteger)
    play_count: Mapped[int | None] = mapped_column(BigInteger)
    danmaku_count: Mapped[int | None] = mapped_column(BigInteger)


class ContentThread(Base):
    """Links a comment to its post and, for replies, to its parent comment."""

    __tablename__ = "content_threads"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "content_id"],
            ["content_records.owner_id", "content_records.id"],
            ondelete="CASCADE",
            name="content_threads_owner_content_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "post_content_id"],
            ["content_records.owner_id", "content_records.id"],
            ondelete="CASCADE",
            name="content_threads_owner_post_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "parent_content_id"],
            ["content_records.owner_id", "content_records.id"],
            ondelete="CASCADE",
            name="content_threads_owner_parent_fkey",
        ),
        CheckConstraint(
            "content_id <> post_content_id "
            "AND (parent_content_id IS NULL OR parent_content_id <> content_id)",
            name="content_threads_distinct_check",
        ),
        Index("content_threads_post_idx", "owner_id", "post_content_id", "content_id"),
    )

    owner_id: Mapped[UUID] = mapped_column(primary_key=True)
    content_id: Mapped[UUID] = mapped_column(primary_key=True)
    post_content_id: Mapped[UUID]
    parent_content_id: Mapped[UUID | None]
    created_at: Mapped[datetime]


class ContentVisibilityObservation(Base):
    __tablename__ = "content_visibility_observations"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "content_id"],
            ["content_records.owner_id", "content_records.id"],
            ondelete="CASCADE",
            name="content_visibility_observations_owner_content_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "job_id"],
            ["jobs.owner_id", "jobs.id"],
            ondelete="RESTRICT",
            name="content_visibility_observations_owner_job_fkey",
        ),
        UniqueConstraint(
            "owner_id",
            "content_id",
            "source_operation_id",
            name="content_visibility_observations_owner_content_operation_key",
        ),
        CheckConstraint(
            "received_at >= observed_at",
            name="content_visibility_observations_received_at_check",
        ),
        CheckConstraint(
            "(status = 'visible' AND basis = 'content_returned') OR "
            "(status = 'deleted' AND basis IN ('source_tombstone', 'http_gone')) OR "
            "(status = 'restricted' AND basis IN "
            "('access_denied', 'authentication_required')) OR "
            "(status = 'transient_failure' AND basis IN "
            "('timeout', 'rate_limited', 'upstream_error')) OR "
            "(status = 'unknown' AND basis IN ('not_found', 'protocol_error'))",
            name="content_visibility_observations_status_basis_check",
        ),
        Index(
            "content_visibility_observations_latest_idx",
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
    status: Mapped[str] = mapped_column(String(32))
    basis: Mapped[str] = mapped_column(String(32))
