from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    ForeignKey,
    ForeignKeyConstraint,
    Index,
    Integer,
    LargeBinary,
    String,
    UniqueConstraint,
    text,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class SourceAccessPolicy(Base):
    __tablename__ = "source_access_policies"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "id",
            name="source_access_policies_owner_id_id_key",
        ),
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


class RetentionPolicy(Base):
    __tablename__ = "evidence_retention_policies"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "source_policy_id"],
            ["source_access_policies.owner_id", "source_access_policies.id"],
            ondelete="CASCADE",
        ),
        UniqueConstraint(
            "owner_id",
            "id",
            name="evidence_retention_policies_owner_id_id_key",
        ),
        UniqueConstraint(
            "owner_id",
            "source_policy_id",
            "data_class",
            name="evidence_retention_policies_owner_source_class_key",
        ),
        CheckConstraint(
            "data_class IN ('structured', 'raw', 'media')",
            name="evidence_retention_policies_data_class_check",
        ),
        CheckConstraint(
            "requested_days BETWEEN 0 AND 3650",
            name="evidence_retention_policies_requested_days_check",
        ),
        CheckConstraint(
            "source_max_days IS NULL OR source_max_days BETWEEN 0 AND 3650",
            name="evidence_retention_policies_source_max_days_check",
        ),
        CheckConstraint(
            "effective_days = CASE WHEN source_max_days IS NULL THEN requested_days "
            "ELSE LEAST(requested_days, source_max_days) END",
            name="evidence_retention_policies_effective_days_check",
        ),
        CheckConstraint(
            "source_policy_version >= 1 AND policy_version >= 1",
            name="evidence_retention_policies_versions_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="evidence_retention_policies_updated_at_check",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    source_policy_id: Mapped[UUID]
    source_policy_version: Mapped[int] = mapped_column(Integer)
    data_class: Mapped[str] = mapped_column(String(16))
    requested_days: Mapped[int] = mapped_column(Integer)
    source_max_days: Mapped[int | None] = mapped_column(Integer)
    effective_days: Mapped[int] = mapped_column(Integer)
    policy_version: Mapped[int] = mapped_column(Integer, server_default=text("1"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class EvidenceResource(Base):
    __tablename__ = "evidence_resources"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "source_policy_id"],
            ["source_access_policies.owner_id", "source_access_policies.id"],
            ondelete="RESTRICT",
        ),
        ForeignKeyConstraint(
            ["owner_id", "retention_policy_id"],
            ["evidence_retention_policies.owner_id", "evidence_retention_policies.id"],
            ondelete="RESTRICT",
        ),
        UniqueConstraint(
            "owner_id",
            "id",
            name="evidence_resources_owner_id_id_key",
        ),
        UniqueConstraint(
            "owner_id",
            "resource_type",
            "resource_id",
            name="evidence_resources_owner_type_resource_key",
        ),
        CheckConstraint(
            "resource_type ~ '^[a-z][a-z0-9_]{0,63}$'",
            name="evidence_resources_resource_type_check",
        ),
        CheckConstraint(
            "source_policy_version >= 1 AND retention_policy_version >= 1",
            name="evidence_resources_versions_check",
        ),
        CheckConstraint(
            "data_class IN ('structured', 'raw', 'media')",
            name="evidence_resources_data_class_check",
        ),
        CheckConstraint(
            "expires_at >= collected_at",
            name="evidence_resources_expiry_check",
        ),
        CheckConstraint(
            "jsonb_typeof(cleanup_targets) = 'array'",
            name="evidence_resources_cleanup_targets_check",
        ),
        Index("evidence_resources_expiry_idx", "expires_at"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    resource_type: Mapped[str] = mapped_column(String(64))
    resource_id: Mapped[UUID]
    source_policy_id: Mapped[UUID]
    source_policy_version: Mapped[int] = mapped_column(Integer)
    retention_policy_id: Mapped[UUID]
    retention_policy_version: Mapped[int] = mapped_column(Integer)
    data_class: Mapped[str] = mapped_column(String(16))
    collected_at: Mapped[datetime]
    expires_at: Mapped[datetime]
    cleanup_targets: Mapped[list[dict[str, str]]] = mapped_column(
        JSONB,
        server_default=text("'[]'::jsonb"),
    )
    created_at: Mapped[datetime]


class DeletionDirective(Base):
    __tablename__ = "evidence_deletions"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "resource_record_id"],
            ["evidence_resources.owner_id", "evidence_resources.id"],
            ondelete="CASCADE",
        ),
        UniqueConstraint(
            "owner_id",
            "operation_id",
            name="evidence_deletions_owner_operation_key",
        ),
        UniqueConstraint(
            "owner_id",
            "resource_record_id",
            name="evidence_deletions_owner_resource_key",
        ),
        CheckConstraint(
            "reason IN ('user_request', 'retention_expired', "
            "'authorization_revoked', 'source_deleted')",
            name="evidence_deletions_reason_check",
        ),
        CheckConstraint(
            "status IN ('pending', 'completed', 'failed')",
            name="evidence_deletions_status_check",
        ),
        CheckConstraint(
            "cleanup_due_at >= requested_at",
            name="evidence_deletions_cleanup_due_check",
        ),
        CheckConstraint(
            "(status = 'completed') = (completed_at IS NOT NULL)",
            name="evidence_deletions_completed_at_check",
        ),
        Index("evidence_deletions_status_due_idx", "status", "cleanup_due_at"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    operation_id: Mapped[UUID]
    resource_record_id: Mapped[UUID]
    reason: Mapped[str] = mapped_column(String(32))
    status: Mapped[str] = mapped_column(String(16))
    requested_at: Mapped[datetime]
    cleanup_due_at: Mapped[datetime]
    completed_at: Mapped[datetime | None]


class CleanupTarget(Base):
    __tablename__ = "evidence_cleanup_targets"
    __table_args__ = (
        UniqueConstraint(
            "deletion_id",
            "target_kind",
            "target_reference",
            name="evidence_cleanup_targets_deletion_kind_reference_key",
        ),
        CheckConstraint(
            "target_kind IN ('redis_cache', 'minio_object')",
            name="evidence_cleanup_targets_kind_check",
        ),
        CheckConstraint(
            "status IN ('pending', 'processing', 'failed', 'succeeded')",
            name="evidence_cleanup_targets_status_check",
        ),
        CheckConstraint(
            "attempt_count BETWEEN 0 AND 6",
            name="evidence_cleanup_targets_attempt_count_check",
        ),
        CheckConstraint(
            "(status = 'processing') = (lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)",
            name="evidence_cleanup_targets_lease_check",
        ),
        CheckConstraint(
            "(status = 'succeeded') = (completed_at IS NOT NULL)",
            name="evidence_cleanup_targets_completed_at_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="evidence_cleanup_targets_updated_at_check",
        ),
        Index("evidence_cleanup_targets_claim_idx", "status", "next_attempt_at"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    deletion_id: Mapped[UUID] = mapped_column(
        ForeignKey("evidence_deletions.id", ondelete="CASCADE")
    )
    target_kind: Mapped[str] = mapped_column(String(32))
    target_reference: Mapped[str] = mapped_column(String(1024))
    status: Mapped[str] = mapped_column(String(16))
    attempt_count: Mapped[int] = mapped_column(Integer, server_default=text("0"))
    next_attempt_at: Mapped[datetime | None]
    lease_token: Mapped[UUID | None]
    lease_expires_at: Mapped[datetime | None]
    last_error_code: Mapped[str | None] = mapped_column(String(128))
    completed_at: Mapped[datetime | None]
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class ProvenanceManifest(Base):
    __tablename__ = "provenance_manifests"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "job_id"],
            ["jobs.owner_id", "jobs.id"],
            ondelete="CASCADE",
        ),
        UniqueConstraint("owner_id", "id", name="provenance_manifests_owner_id_id_key"),
        UniqueConstraint(
            "owner_id",
            "job_id",
            "result_kind",
            name="provenance_manifests_owner_job_result_key",
        ),
        CheckConstraint(
            "result_kind ~ '^[a-z][a-z0-9_.:-]{0,63}$'",
            name="provenance_manifests_result_kind_check",
        ),
        CheckConstraint(
            "method_key ~ '^[a-z][a-z0-9_.:-]{0,127}$'",
            name="provenance_manifests_method_key_check",
        ),
        CheckConstraint(
            "method_version ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'",
            name="provenance_manifests_method_version_check",
        ),
        CheckConstraint(
            "jsonb_typeof(method_parameters) = 'object'",
            name="provenance_manifests_parameters_check",
        ),
        CheckConstraint(
            "octet_length(manifest_fingerprint) = 32",
            name="provenance_manifests_fingerprint_check",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    job_id: Mapped[UUID]
    operation_id: Mapped[UUID]
    result_kind: Mapped[str] = mapped_column(String(64))
    method_key: Mapped[str] = mapped_column(String(128))
    method_version: Mapped[str] = mapped_column(String(128))
    method_parameters: Mapped[dict[str, str | int | bool | None]] = mapped_column(JSONB)
    manifest_fingerprint: Mapped[bytes] = mapped_column(LargeBinary(32))
    created_at: Mapped[datetime]


class ProvenanceManifestItem(Base):
    __tablename__ = "provenance_manifest_inputs"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_id", "manifest_id"],
            ["provenance_manifests.owner_id", "provenance_manifests.id"],
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ["owner_id", "resource_record_id"],
            ["evidence_resources.owner_id", "evidence_resources.id"],
        ),
        UniqueConstraint(
            "manifest_id",
            "role",
            "ordinal",
            name="provenance_manifest_inputs_role_ordinal_key",
        ),
        UniqueConstraint(
            "manifest_id",
            "role",
            "resource_record_id",
            "snapshot_ref",
            name="provenance_manifest_inputs_resource_snapshot_key",
        ),
        CheckConstraint(
            "role IN ('subject', 'reference')",
            name="provenance_manifest_inputs_role_check",
        ),
        CheckConstraint(
            "snapshot_ref ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'",
            name="provenance_manifest_inputs_snapshot_check",
        ),
        CheckConstraint("ordinal >= 0", name="provenance_manifest_inputs_ordinal_check"),
        Index("provenance_manifest_inputs_manifest_idx", "manifest_id", "role", "ordinal"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    manifest_id: Mapped[UUID]
    role: Mapped[str] = mapped_column(String(16))
    resource_record_id: Mapped[UUID]
    snapshot_ref: Mapped[str] = mapped_column(String(128))
    ordinal: Mapped[int] = mapped_column(Integer)
