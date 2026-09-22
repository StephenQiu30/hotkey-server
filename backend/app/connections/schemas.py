from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from typing import Self
from uuid import UUID

from pydantic import Field, field_validator, model_validator

from core.schemas import InputModel, OutputModel
from sources.contracts import SourceCapability, SourceStopReason


class SourceRolloutRole(StrEnum):
    REQUIRED = "required"
    CANDIDATE = "candidate"


class SourceConnectionStatus(StrEnum):
    ACTIVE = "active"
    DISABLED = "disabled"


class SourceEntryPoint(StrEnum):
    MANUAL = "manual"
    SCHEDULED = "scheduled"


class ConnectionEvidenceKind(StrEnum):
    PROBE = "probe"
    PERSISTED_READ = "persisted_read"


class ConnectionEvidenceOutcome(StrEnum):
    SUCCEEDED = "succeeded"
    FAILED = "failed"


class _SourceCapabilityEvidenceInput(InputModel):
    operation_id: UUID
    connection_id: UUID
    connection_version: int = Field(ge=1)
    capability: SourceCapability
    entry_point: SourceEntryPoint
    outcome: ConnectionEvidenceOutcome
    stop_reason: SourceStopReason | None
    component_name: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$",
    )
    component_version: str = Field(
        min_length=1,
        max_length=64,
        pattern=r"^[A-Za-z0-9][A-Za-z0-9._+:-]{0,63}$",
    )

    @model_validator(mode="after")
    def validate_outcome_reason(self) -> Self:
        if self.outcome is ConnectionEvidenceOutcome.SUCCEEDED and self.stop_reason is not None:
            raise ValueError("stop_reason must be absent for succeeded evidence")
        if self.outcome is ConnectionEvidenceOutcome.FAILED and self.stop_reason is None:
            raise ValueError("stop_reason is required for failed evidence")
        return self


class ProbeEvidenceInput(_SourceCapabilityEvidenceInput):
    pass


class PersistedReadEvidenceInput(_SourceCapabilityEvidenceInput):
    resource_ref: str | None = Field(default=None, min_length=1, max_length=512)

    @field_validator("resource_ref")
    @classmethod
    def validate_resource_ref(cls, value: str | None) -> str | None:
        if value is not None and (
            value != value.strip()
            or any(ord(character) < 32 or ord(character) == 127 for character in value)
        ):
            raise ValueError("resource_ref cannot contain whitespace padding or controls")
        return value

    @model_validator(mode="after")
    def validate_resource_for_outcome(self) -> Self:
        if self.outcome is ConnectionEvidenceOutcome.SUCCEEDED and self.resource_ref is None:
            raise ValueError("resource_ref is required for succeeded persisted reads")
        if self.outcome is ConnectionEvidenceOutcome.FAILED and self.resource_ref is not None:
            raise ValueError("resource_ref must be absent for failed persisted reads")
        return self


class SourceCapabilityEvidenceView(OutputModel):
    id: UUID
    operation_id: UUID
    connection_id: UUID
    connection_version: int
    capability: SourceCapability
    entry_point: SourceEntryPoint
    kind: ConnectionEvidenceKind
    outcome: ConnectionEvidenceOutcome
    stop_reason: SourceStopReason | None
    component_name: str
    component_version: str
    observed_at: datetime


class SourceCapabilityStatus(StrEnum):
    UNCONFIGURED = "unconfigured"
    PENDING_VERIFICATION = "pending_verification"
    AVAILABLE = "available"
    AUTHENTICATION_REQUIRED = "authentication_required"
    RESTRICTED = "restricted"
    DISABLED = "disabled"


class SourcePlatformStatus(StrEnum):
    UNCONFIGURED = "unconfigured"
    PENDING_VERIFICATION = "pending_verification"
    AVAILABLE = "available"
    AUTHENTICATION_REQUIRED = "authentication_required"
    RESTRICTED = "restricted"
    DISABLED = "disabled"
    PARTIAL = "partial"


class SourceEntryPointView(OutputModel):
    status: SourceCapabilityStatus
    last_checked_at: datetime | None
    last_persisted_success_at: datetime | None
    stop_reason: SourceStopReason | None
    next_action: str


class SourceCapabilityView(OutputModel):
    capability: SourceCapability
    display_name: str
    manual: SourceEntryPointView
    scheduled: SourceEntryPointView


class SourcePlatformView(OutputModel):
    source_key: str
    display_name: str
    rollout_role: SourceRolloutRole
    status: SourcePlatformStatus
    connection_version: int | None
    has_credentials: bool
    capabilities: list[SourceCapabilityView]
