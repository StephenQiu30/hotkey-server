from __future__ import annotations

from datetime import datetime
from enum import StrEnum

from core.schemas import OutputModel
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
