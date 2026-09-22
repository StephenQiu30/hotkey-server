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
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class SourceConnection(Base):
    __tablename__ = "source_connections"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "source_key",
            name="source_connections_owner_source_key",
        ),
        UniqueConstraint(
            "owner_id",
            "id",
            name="source_connections_owner_id_key",
        ),
        ForeignKeyConstraint(
            ["owner_id", "id", "current_version"],
            [
                "source_connection_versions.owner_id",
                "source_connection_versions.connection_id",
                "source_connection_versions.version",
            ],
            name="source_connections_current_version_fkey",
            deferrable=True,
            initially="DEFERRED",
            use_alter=True,
        ),
        CheckConstraint(
            "source_key ~ '^[a-z][a-z0-9_-]{0,63}$'",
            name="source_connections_source_key_check",
        ),
        CheckConstraint(
            "status IN ('active', 'disabled')",
            name="source_connections_status_check",
        ),
        CheckConstraint(
            "current_version >= 1",
            name="source_connections_current_version_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="source_connections_updated_at_check",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    source_key: Mapped[str] = mapped_column(String(64))
    status: Mapped[str] = mapped_column(String(16))
    current_version: Mapped[int] = mapped_column(Integer, server_default=text("1"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class SourceConnectionVersion(Base):
    __tablename__ = "source_connection_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "connection_id"],
            ["source_connections.owner_id", "source_connections.id"],
            ondelete="CASCADE",
            name="source_connection_versions_owner_connection_fkey",
        ),
        UniqueConstraint(
            "owner_id",
            "connection_id",
            "version",
            name="source_connection_versions_owner_connection_version_key",
        ),
        CheckConstraint(
            "version >= 1",
            name="source_connection_versions_version_check",
        ),
        CheckConstraint(
            "auth_kind IN ('none', 'server_credential')",
            name="source_connection_versions_auth_kind_check",
        ),
        CheckConstraint(
            "(auth_kind = 'none' AND secret_ref IS NULL) OR "
            "(auth_kind = 'server_credential' AND secret_ref IS NOT NULL AND "
            "secret_ref ~ '^[A-Za-z][A-Za-z0-9+.-]*:[A-Za-z0-9_./:-]+$')",
            name="source_connection_versions_auth_secret_check",
        ),
        CheckConstraint(
            "jsonb_typeof(configuration) = 'object'",
            name="source_connection_versions_configuration_check",
        ),
        Index(
            "source_connection_versions_created_by_idx",
            "created_by",
        ),
    )

    connection_id: Mapped[UUID] = mapped_column(primary_key=True)
    version: Mapped[int] = mapped_column(Integer, primary_key=True)
    owner_id: Mapped[UUID]
    auth_kind: Mapped[str] = mapped_column(String(32), server_default=text("'server_credential'"))
    secret_ref: Mapped[str | None] = mapped_column(String(256))
    configuration: Mapped[dict[str, object]] = mapped_column(
        JSONB, server_default=text("'{}'::jsonb")
    )
    created_by: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="RESTRICT"))
    created_at: Mapped[datetime]


class SourceCapabilityEvidence(Base):
    __tablename__ = "source_capability_evidence"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "connection_id", "connection_version"],
            [
                "source_connection_versions.owner_id",
                "source_connection_versions.connection_id",
                "source_connection_versions.version",
            ],
            ondelete="CASCADE",
            name="source_capability_evidence_connection_version_fkey",
        ),
        UniqueConstraint(
            "owner_id",
            "operation_id",
            name="source_capability_evidence_owner_operation_key",
        ),
        CheckConstraint(
            "connection_version >= 1",
            name="source_capability_evidence_connection_version_check",
        ),
        CheckConstraint(
            "capability IN ('search', 'author_posts', 'comments', 'replies', 'page_content')",
            name="source_capability_evidence_capability_check",
        ),
        CheckConstraint(
            "entry_point IN ('manual', 'scheduled')",
            name="source_capability_evidence_entry_point_check",
        ),
        CheckConstraint(
            "kind IN ('probe', 'persisted_read')",
            name="source_capability_evidence_kind_check",
        ),
        CheckConstraint(
            "outcome IN ('succeeded', 'failed')",
            name="source_capability_evidence_outcome_check",
        ),
        CheckConstraint(
            "stop_reason IS NULL OR stop_reason IN ("
            "'end_of_results', 'source_empty', 'rate_limited', "
            "'authentication_required', 'access_denied', 'not_found', "
            "'unsupported', 'cancelled', 'budget_exhausted', "
            "'upstream_error', 'protocol_error')",
            name="source_capability_evidence_stop_reason_check",
        ),
        CheckConstraint(
            "(outcome = 'succeeded' AND stop_reason IS NULL) OR "
            "(outcome = 'failed' AND stop_reason IS NOT NULL)",
            name="source_capability_evidence_outcome_reason_check",
        ),
        CheckConstraint(
            "kind <> 'probe' OR resource_ref IS NULL",
            name="source_capability_evidence_probe_resource_check",
        ),
        CheckConstraint(
            "kind <> 'persisted_read' OR outcome <> 'succeeded' OR resource_ref IS NOT NULL",
            name="source_capability_evidence_persisted_resource_check",
        ),
        Index(
            "source_capability_evidence_latest_idx",
            "owner_id",
            "connection_id",
            "connection_version",
            "capability",
            "entry_point",
            "observed_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    operation_id: Mapped[UUID]
    owner_id: Mapped[UUID]
    connection_id: Mapped[UUID]
    connection_version: Mapped[int] = mapped_column(Integer)
    capability: Mapped[str] = mapped_column(String(32))
    entry_point: Mapped[str] = mapped_column(String(16))
    kind: Mapped[str] = mapped_column(String(32))
    outcome: Mapped[str] = mapped_column(String(16))
    stop_reason: Mapped[str | None] = mapped_column(String(32))
    resource_ref: Mapped[str | None] = mapped_column(String(512))
    component_name: Mapped[str] = mapped_column(String(128))
    component_version: Mapped[str] = mapped_column(String(64))
    observed_at: Mapped[datetime]
    created_at: Mapped[datetime]
