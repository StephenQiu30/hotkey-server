from __future__ import annotations

import re
from datetime import datetime
from enum import StrEnum
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

type AdmittedScalar = str | int | float | bool | None
type AdmittedValue = AdmittedScalar | list[AdmittedScalar]

_FIELD_NAME_PATTERN = re.compile(r"^[a-z][a-z0-9_]{0,63}$")
_MAX_FIELDS = 64
_SECRET_FIELD_NAMES = frozenset(
    {
        "authorization",
        "access_token",
        "api_key",
        "client_secret",
        "cookie",
        "csrf_token",
        "password",
        "refresh_token",
        "secret",
        "session_token",
        "token",
    }
)


class SourceCapability(StrEnum):
    SEARCH = "search"
    AUTHOR_POSTS = "author_posts"
    COMMENTS = "comments"
    REPLIES = "replies"


class AccessPolicyStatus(StrEnum):
    PENDING = "pending"
    APPROVED = "approved"
    BLOCKED = "blocked"


class AccessBasis(StrEnum):
    OFFICIAL_API = "official_api"
    AUTHORIZED_FEED = "authorized_feed"
    WRITTEN_PERMISSION = "written_permission"
    MANUAL_IMPORT = "manual_import"


class DataClass(StrEnum):
    STRUCTURED = "structured"
    RAW = "raw"
    MEDIA = "media"


class CleanupTargetKind(StrEnum):
    REDIS_CACHE = "redis_cache"
    MINIO_OBJECT = "minio_object"


class DeletionReason(StrEnum):
    USER_REQUEST = "user_request"
    RETENTION_EXPIRED = "retention_expired"
    AUTHORIZATION_REVOKED = "authorization_revoked"
    SOURCE_DELETED = "source_deleted"


class DeletionStatus(StrEnum):
    PENDING = "pending"
    COMPLETED = "completed"
    FAILED = "failed"


class CleanupStatus(StrEnum):
    PENDING = "pending"
    PROCESSING = "processing"
    FAILED = "failed"
    SUCCEEDED = "succeeded"


class SourceAccessPolicyInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    source_key: str = Field(
        min_length=1,
        max_length=64,
        pattern=r"^[a-z][a-z0-9_-]{0,63}$",
    )
    capability: SourceCapability
    status: AccessPolicyStatus
    enabled: bool = False
    access_basis: AccessBasis | None = None
    terms_reference: str | None = Field(default=None, min_length=1, max_length=512)
    processing_purpose: str = Field(min_length=1, max_length=256)
    component_name: str | None = Field(default=None, min_length=1, max_length=128)
    component_version: str | None = Field(default=None, min_length=1, max_length=64)
    component_license: str | None = Field(default=None, min_length=1, max_length=128)
    field_purposes: dict[str, str] = Field(default_factory=dict)
    reviewed_at: datetime | None = None
    review_expires_at: datetime | None = None

    @field_validator("processing_purpose")
    @classmethod
    def validate_processing_purpose(cls, value: str) -> str:
        if value != value.strip():
            raise ValueError("processing_purpose cannot have surrounding whitespace")
        return value

    @field_validator("reviewed_at", "review_expires_at")
    @classmethod
    def validate_timezone(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.tzinfo is None:
            raise ValueError("review timestamps must be timezone-aware")
        return value

    @field_validator("field_purposes")
    @classmethod
    def validate_field_purposes(cls, value: dict[str, str]) -> dict[str, str]:
        if len(value) > _MAX_FIELDS:
            raise ValueError(f"field_purposes cannot contain more than {_MAX_FIELDS} items")
        for field_name, purpose in value.items():
            if _FIELD_NAME_PATTERN.fullmatch(field_name) is None:
                raise ValueError("field names must be stable lowercase identifiers")
            if field_name in _SECRET_FIELD_NAMES:
                raise ValueError("secret fields cannot be admitted")
            if purpose != purpose.strip() or not 1 <= len(purpose) <= 256:
                raise ValueError("field purposes must contain between 1 and 256 characters")
        return value

    @model_validator(mode="after")
    def validate_decision(self) -> SourceAccessPolicyInput:
        if self.review_expires_at is not None and (
            self.reviewed_at is None or self.review_expires_at <= self.reviewed_at
        ):
            raise ValueError("review_expires_at must follow reviewed_at")
        if self.enabled and self.status is not AccessPolicyStatus.APPROVED:
            raise ValueError("only approved policies can be enabled")
        if self.status is AccessPolicyStatus.APPROVED:
            required = (
                self.access_basis,
                self.terms_reference,
                self.component_name,
                self.component_version,
                self.component_license,
                self.reviewed_at,
            )
            if any(item is None for item in required) or not self.field_purposes:
                raise ValueError("approved policies require review evidence and field purposes")
        return self


class SourceAccessPolicyView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    owner_id: UUID
    source_key: str
    capability: SourceCapability
    status: AccessPolicyStatus
    enabled: bool
    access_basis: AccessBasis | None
    terms_reference: str | None
    processing_purpose: str
    component_name: str | None
    component_version: str | None
    component_license: str | None
    field_purposes: dict[str, str]
    reviewed_at: datetime | None
    review_expires_at: datetime | None
    policy_version: int
    created_at: datetime
    updated_at: datetime


class RetentionPolicyInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    source_policy_id: UUID
    data_class: DataClass
    requested_days: int = Field(ge=0, le=3650)
    source_max_days: int | None = Field(default=None, ge=0, le=3650)


class RetentionPolicyView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    owner_id: UUID
    source_policy_id: UUID
    source_policy_version: int
    data_class: DataClass
    requested_days: int
    source_max_days: int | None
    effective_days: int
    policy_version: int
    created_at: datetime
    updated_at: datetime


class CleanupTargetSpec(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    kind: CleanupTargetKind
    reference: str = Field(min_length=1, max_length=1024)

    @model_validator(mode="after")
    def validate_reference(self) -> CleanupTargetSpec:
        reference = self.reference
        if reference != reference.strip() or any(
            ord(character) < 32 or ord(character) == 127 for character in reference
        ):
            raise ValueError("cleanup target reference contains unsafe whitespace")
        if self.kind is CleanupTargetKind.REDIS_CACHE and not reference.startswith("hotkey:"):
            raise ValueError("Redis cleanup targets must use the hotkey namespace")
        if self.kind is CleanupTargetKind.MINIO_OBJECT:
            parts = reference.split("/")
            if reference.startswith("/") or any(part in {"", ".", ".."} for part in parts):
                raise ValueError("MinIO cleanup target must be a normalized object name")
            if len(reference.encode()) > 1024:
                raise ValueError("MinIO object name cannot exceed 1024 bytes")
        return self


class AdmittedSourcePayload(BaseModel):
    model_config = ConfigDict(frozen=True)

    policy_id: UUID
    policy_version: int
    owner_id: UUID
    source_key: str
    capability: SourceCapability
    retention_policy_id: UUID
    retention_policy_version: int
    data_class: DataClass
    collected_at: datetime
    expires_at: datetime
    fields: dict[str, AdmittedValue]


class EvidenceResourceView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    owner_id: UUID
    resource_type: str
    resource_id: UUID
    source_policy_id: UUID
    source_policy_version: int
    retention_policy_id: UUID
    retention_policy_version: int
    data_class: DataClass
    collected_at: datetime
    expires_at: datetime
    cleanup_targets: tuple[CleanupTargetSpec, ...]
    created_at: datetime


class DeletionView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    owner_id: UUID
    operation_id: UUID
    resource_record_id: UUID
    resource_type: str
    resource_id: UUID
    reason: DeletionReason
    status: DeletionStatus
    requested_at: datetime
    cleanup_due_at: datetime
    completed_at: datetime | None
    target_count: int
    completed_targets: int
    failed_targets: int


class CleanupLease(BaseModel):
    model_config = ConfigDict(frozen=True)

    target_id: UUID
    deletion_id: UUID
    kind: CleanupTargetKind
    reference: str
    lease_token: UUID
    attempt_count: int
    lease_expires_at: datetime


class CleanupBatchResult(BaseModel):
    model_config = ConfigDict(frozen=True)

    succeeded: int = 0
    failed: int = 0
