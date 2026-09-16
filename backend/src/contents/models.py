from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class Content(Base):
    __tablename__ = "contents"
    __table_args__ = (
        UniqueConstraint(
            "source",
            "provider_namespace",
            "external_id",
            name="uq_contents_provider_identity",
        ),
        CheckConstraint("kind IN ('post', 'comment', 'reply')", name="ck_contents_kind"),
        CheckConstraint(
            "visibility IN ('available', 'unavailable', 'deleted')",
            name="ck_contents_visibility",
        ),
        CheckConstraint(
            "relation_status IN ('root', 'unresolved', 'resolved')",
            name="ck_contents_relation_status",
        ),
        Index("ix_contents_inbox_order", "first_seen_at", "id"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    source: Mapped[str] = mapped_column(String(32), index=True)
    provider_namespace: Mapped[str] = mapped_column(String(100))
    external_id: Mapped[str] = mapped_column(String(1024))
    kind: Mapped[str] = mapped_column(String(16))
    canonical_url: Mapped[str | None] = mapped_column(String(2048))
    author_ref: Mapped[str] = mapped_column(String(1024))
    root_external_id: Mapped[str] = mapped_column(String(1024))
    parent_external_id: Mapped[str | None] = mapped_column(String(1024))
    relation_status: Mapped[str] = mapped_column(String(16))
    visibility: Mapped[str] = mapped_column(String(16))
    first_seen_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    last_seen_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), index=True)


class ContentVersion(Base):
    __tablename__ = "content_versions"
    __table_args__ = (
        UniqueConstraint("content_id", "version", name="uq_content_versions_number"),
        UniqueConstraint("content_id", "text_sha256", name="uq_content_versions_text"),
        CheckConstraint("version > 0", name="ck_content_versions_version"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    content_id: Mapped[UUID] = mapped_column(
        ForeignKey("contents.id", ondelete="CASCADE"), index=True
    )
    version: Mapped[int] = mapped_column(Integer)
    text: Mapped[str] = mapped_column(Text)
    text_sha256: Mapped[str] = mapped_column(String(64))
    published_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    observed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    raw_page_id: Mapped[UUID] = mapped_column(ForeignKey("raw_pages.id"))


class ContentObservation(Base):
    __tablename__ = "content_observations"
    __table_args__ = (
        UniqueConstraint("content_id", "raw_page_id", name="uq_content_observations_page"),
        CheckConstraint(
            "reply_count IS NULL OR reply_count >= 0", name="ck_content_observations_reply_count"
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    content_id: Mapped[UUID] = mapped_column(
        ForeignKey("contents.id", ondelete="CASCADE"), index=True
    )
    observed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), index=True)
    reply_count: Mapped[int | None] = mapped_column(Integer)
    raw_page_id: Mapped[UUID] = mapped_column(ForeignKey("raw_pages.id"))


class ContentWithdrawalRecord(Base):
    __tablename__ = "content_withdrawal_records"
    __table_args__ = (
        UniqueConstraint(
            "source",
            "provider_namespace",
            "external_id",
            name="uq_content_withdrawal_records_identity",
        ),
        CheckConstraint(
            "visibility IN ('unavailable', 'deleted')",
            name="ck_content_withdrawal_records_visibility",
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    source: Mapped[str] = mapped_column(String(32))
    provider_namespace: Mapped[str] = mapped_column(String(100))
    external_id: Mapped[str] = mapped_column(String(1024))
    visibility: Mapped[str] = mapped_column(String(16))
    requested_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
