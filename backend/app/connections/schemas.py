from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from typing import Self
from uuid import UUID

from pydantic import Field, HttpUrl, field_validator, model_validator

from core.schemas import InputModel, OutputModel
from sources.adapters.web_targets import normalize_web_host
from sources.contracts import SourceCapability, SourceStopReason

_LOCAL_SOURCE_CONFIG_HOSTS = frozenset({"127.0.0.1", "localhost"})


def _normalize_source_config_host(value: str) -> str:
    if value in _LOCAL_SOURCE_CONFIG_HOSTS:
        return value
    return normalize_web_host(value)


class SourceRolloutRole(StrEnum):
    REQUIRED = "required"
    CANDIDATE = "candidate"


class SourceConnectionStatus(StrEnum):
    ACTIVE = "active"
    DISABLED = "disabled"


class SourceConnectionAuthKind(StrEnum):
    NONE = "none"
    SERVER_CREDENTIAL = "server_credential"
    BROWSER_STATE = "browser_state"


class SourceConnectionConfig(InputModel):
    feed_url_template: str | None = Field(default=None, min_length=1, max_length=2048)
    base_url: HttpUrl | None = None
    engines: tuple[str, ...] = Field(default=(), max_length=16)
    allowed_hosts: tuple[str, ...] = Field(default=(), max_length=32)

    @field_validator("feed_url_template")
    @classmethod
    def validate_feed_url_template(cls, value: str | None) -> str | None:
        if value is not None and (
            value != value.strip()
            or any(ord(character) < 32 or ord(character) == 127 for character in value)
        ):
            raise ValueError("feed_url_template cannot contain padding or controls")
        return value

    @field_validator("engines")
    @classmethod
    def validate_engines(cls, value: tuple[str, ...]) -> tuple[str, ...]:
        normalized = tuple(item.strip() for item in value)
        if any(not item or len(item) > 64 for item in normalized) or len(set(normalized)) != len(
            normalized
        ):
            raise ValueError("engines must contain unique non-empty names")
        return normalized

    @field_validator("allowed_hosts")
    @classmethod
    def validate_config_allowed_hosts(cls, value: tuple[str, ...]) -> tuple[str, ...]:
        normalized = tuple(sorted(_normalize_source_config_host(item) for item in value))
        if len(set(normalized)) != len(normalized):
            raise ValueError("allowed_hosts must contain unique exact domains")
        return normalized

    @model_validator(mode="after")
    def validate_non_secret_base_url(self) -> Self:
        if self.base_url is None:
            return self
        if self.base_url.username is not None or self.base_url.password is not None:
            raise ValueError("base_url cannot contain credentials")
        if self.base_url.host is None:
            raise ValueError("base_url requires a host")
        host = _normalize_source_config_host(self.base_url.host)
        if self.allowed_hosts and host not in self.allowed_hosts:
            raise ValueError("base_url host must be included in allowed_hosts")
        return self


class SourceEntryPoint(StrEnum):
    MANUAL = "manual"
    SCHEDULED = "scheduled"


class SourceConnectionUpdateInput(InputModel):
    expected_version: int = Field(ge=0, le=2_147_483_646)
    status: SourceConnectionStatus
    allowed_hosts: tuple[str, ...] = Field(default=(), max_length=32)

    @field_validator("allowed_hosts")
    @classmethod
    def validate_allowed_hosts(cls, value: tuple[str, ...]) -> tuple[str, ...]:
        normalized = tuple(sorted(normalize_web_host(item) for item in value))
        if len(set(normalized)) != len(normalized):
            raise ValueError("allowed_hosts must contain unique exact domains")
        return normalized


class SourceConnectionView(OutputModel):
    id: UUID
    source_key: str
    status: SourceConnectionStatus
    version: int
    allowed_hosts: list[str]
    updated_at: datetime


class SourcePresetApplyView(OutputModel):
    source_key: str
    connection_id: UUID
    connection_version: int
    capabilities: tuple[SourceCapability, ...]


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
    connection_id: UUID | None
    connection_status: SourceConnectionStatus | None
    credential_configured: bool
    credential_update_available: bool
    allowed_hosts: list[str]
    capabilities: list[SourceCapabilityView]
