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


class AdmittedSourcePayload(BaseModel):
    model_config = ConfigDict(frozen=True)

    policy_id: UUID
    policy_version: int
    owner_id: UUID
    source_key: str
    capability: SourceCapability
    fields: dict[str, AdmittedValue]
