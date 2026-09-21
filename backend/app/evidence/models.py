from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import Boolean, CheckConstraint, ForeignKey, Integer, String, UniqueConstraint, text
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class SourceAccessPolicy(Base):
    __tablename__ = "source_access_policies"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "source_key",
            "capability",
            name="source_access_policies_owner_source_capability_key",
        ),
        CheckConstraint(
            "source_key ~ '^[a-z][a-z0-9_-]{0,63}$'",
            name="source_access_policies_source_key_check",
        ),
        CheckConstraint(
            "capability IN ('search', 'author_posts', 'comments', 'replies')",
            name="source_access_policies_capability_check",
        ),
        CheckConstraint(
            "status IN ('pending', 'approved', 'blocked')",
            name="source_access_policies_status_check",
        ),
        CheckConstraint(
            "access_basis IS NULL OR access_basis IN "
            "('official_api', 'authorized_feed', 'written_permission', 'manual_import')",
            name="source_access_policies_access_basis_check",
        ),
        CheckConstraint(
            "jsonb_typeof(field_purposes) = 'object'",
            name="source_access_policies_field_purposes_check",
        ),
        CheckConstraint(
            "NOT enabled OR status = 'approved'",
            name="source_access_policies_enabled_status_check",
        ),
        CheckConstraint(
            "status <> 'approved' OR ("
            "access_basis IS NOT NULL AND terms_reference IS NOT NULL "
            "AND component_name IS NOT NULL AND component_version IS NOT NULL "
            "AND component_license IS NOT NULL AND reviewed_at IS NOT NULL "
            "AND field_purposes <> '{}'::jsonb)",
            name="source_access_policies_approved_evidence_check",
        ),
        CheckConstraint(
            "review_expires_at IS NULL OR "
            "(reviewed_at IS NOT NULL AND review_expires_at > reviewed_at)",
            name="source_access_policies_review_window_check",
        ),
        CheckConstraint(
            "policy_version >= 1",
            name="source_access_policies_version_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="source_access_policies_updated_at_check",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    source_key: Mapped[str] = mapped_column(String(64))
    capability: Mapped[str] = mapped_column(String(32))
    status: Mapped[str] = mapped_column(String(16))
    enabled: Mapped[bool] = mapped_column(Boolean, server_default=text("false"))
    access_basis: Mapped[str | None] = mapped_column(String(32))
    terms_reference: Mapped[str | None] = mapped_column(String(512))
    processing_purpose: Mapped[str] = mapped_column(String(256))
    component_name: Mapped[str | None] = mapped_column(String(128))
    component_version: Mapped[str | None] = mapped_column(String(64))
    component_license: Mapped[str | None] = mapped_column(String(128))
    field_purposes: Mapped[dict[str, str]] = mapped_column(
        JSONB,
        server_default=text("'{}'::jsonb"),
    )
    reviewed_at: Mapped[datetime | None]
    review_expires_at: Mapped[datetime | None]
    policy_version: Mapped[int] = mapped_column(Integer, server_default=text("1"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]
