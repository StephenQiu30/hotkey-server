from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from typing import Literal, Self
from uuid import UUID

from pydantic import ConfigDict, Field, field_validator, model_validator

from connections.schemas import SourceEntryPoint
from core.schemas import InputModel, OutputModel
from evidence.schemas import AdmittedSourcePayload, DataClass
from jobs.schemas import CollectionScanKind
from sources.contracts import SourceCapability


def _validate_opaque_identifier(value: str | None, *, field_name: str) -> str | None:
    if value is None:
        return None
    if (
        not value
        or value != value.strip()
        or any(ord(character) < 32 or ord(character) == 127 for character in value)
    ):
        raise ValueError(f"{field_name} must be an opaque non-empty identifier")
    return value


class PersistContentPostInput(InputModel):
    model_config = ConfigDict(extra="forbid", str_strip_whitespace=False)

    job_id: UUID
    source_operation_id: UUID
    connection_id: UUID
    connection_version: int = Field(ge=1)
    entry_point: SourceEntryPoint
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
    native_scope: str | None = Field(default=None, max_length=512)
    admission: AdmittedSourcePayload

    @field_validator("native_scope")
    @classmethod
    def validate_native_scope(cls, value: str | None) -> str | None:
        return _validate_opaque_identifier(value, field_name="native_scope")

    @model_validator(mode="after")
    def validate_admitted_identity_and_time(self) -> Self:
        external_id = self.admission.fields.get("external_id")
        if not isinstance(external_id, str):
            raise ValueError("external_id must be admitted as a string")
        _validate_opaque_identifier(external_id, field_name="external_id")
        if self.admission.collected_at.utcoffset() is None:
            raise ValueError("collected_at must be timezone-aware")
        if self.admission.expires_at.utcoffset() is None:
            raise ValueError("expires_at must be timezone-aware")
        return self


class PersistContentDocumentInput(InputModel):
    model_config = ConfigDict(extra="forbid", str_strip_whitespace=False)

    job_id: UUID
    source_operation_id: UUID
    connection_id: UUID
    connection_version: int = Field(ge=1)
    entry_point: SourceEntryPoint
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
    admission: AdmittedSourcePayload

    @model_validator(mode="after")
    def validate_document_admission(self) -> Self:
        if (
            self.admission.source_key != "web"
            or self.admission.capability is not SourceCapability.PAGE_CONTENT
            or self.admission.data_class is not DataClass.STRUCTURED
        ):
            raise ValueError("document admission must describe structured web page content")
        if self.admission.collected_at.utcoffset() is None:
            raise ValueError("collected_at must be timezone-aware")
        if self.admission.expires_at.utcoffset() is None:
            raise ValueError("expires_at must be timezone-aware")
        return self


class ContentMetricView(OutputModel):
    like_count: int | None = Field(ge=0)
    comment_count: int | None = Field(ge=0)
    repost_count: int | None = Field(ge=0)
    view_count: int | None = Field(ge=0)
    play_count: int | None = Field(ge=0)
    danmaku_count: int | None = Field(ge=0)


class ContentTextScope(StrEnum):
    FULL = "full"
    SUMMARY = "summary"
    TRUNCATED = "truncated"
    MEDIA_ONLY = "media_only"


class ContentTextOrigin(StrEnum):
    SOURCE = "source"
    MACHINE_EXTRACTED = "machine_extracted"


class ContentTruncationReason(StrEnum):
    SOURCE_LIMIT = "source_limit"
    COLLECTOR_LIMIT = "collector_limit"


class ContentRelationType(StrEnum):
    QUOTE = "quote"
    REPOST = "repost"


class ContentVisibilityStatus(StrEnum):
    VISIBLE = "visible"
    DELETED = "deleted"
    RESTRICTED = "restricted"
    TRANSIENT_FAILURE = "transient_failure"
    UNKNOWN = "unknown"


class ContentVisibilityBasis(StrEnum):
    CONTENT_RETURNED = "content_returned"
    SOURCE_TOMBSTONE = "source_tombstone"
    HTTP_GONE = "http_gone"
    ACCESS_DENIED = "access_denied"
    AUTHENTICATION_REQUIRED = "authentication_required"
    NOT_FOUND = "not_found"
    TIMEOUT = "timeout"
    RATE_LIMITED = "rate_limited"
    UPSTREAM_ERROR = "upstream_error"
    PROTOCOL_ERROR = "protocol_error"


_VISIBILITY_BASES = {
    ContentVisibilityStatus.VISIBLE: frozenset({ContentVisibilityBasis.CONTENT_RETURNED}),
    ContentVisibilityStatus.DELETED: frozenset(
        {ContentVisibilityBasis.SOURCE_TOMBSTONE, ContentVisibilityBasis.HTTP_GONE}
    ),
    ContentVisibilityStatus.RESTRICTED: frozenset(
        {ContentVisibilityBasis.ACCESS_DENIED, ContentVisibilityBasis.AUTHENTICATION_REQUIRED}
    ),
    ContentVisibilityStatus.TRANSIENT_FAILURE: frozenset(
        {
            ContentVisibilityBasis.TIMEOUT,
            ContentVisibilityBasis.RATE_LIMITED,
            ContentVisibilityBasis.UPSTREAM_ERROR,
        }
    ),
    ContentVisibilityStatus.UNKNOWN: frozenset(
        {ContentVisibilityBasis.NOT_FOUND, ContentVisibilityBasis.PROTOCOL_ERROR}
    ),
}


class RecordContentVisibilityInput(InputModel):
    content_id: UUID
    job_id: UUID
    source_operation_id: UUID
    observed_at: datetime
    status: ContentVisibilityStatus
    basis: ContentVisibilityBasis

    @field_validator("observed_at")
    @classmethod
    def validate_observed_at(cls, value: datetime) -> datetime:
        if value.utcoffset() is None:
            raise ValueError("observed_at must be timezone-aware")
        return value

    @model_validator(mode="after")
    def validate_status_basis(self) -> Self:
        if self.basis not in _VISIBILITY_BASES[self.status]:
            raise ValueError("basis does not match visibility status")
        return self


class ContentVersionRelationView(OutputModel):
    relation_type: ContentRelationType
    target_native_scope: str | None
    target_external_id: str
    target_author_external_id: str | None
    target_content_id: UUID | None


class ContentVersionView(OutputModel):
    id: UUID
    text_scope: ContentTextScope
    text_origin: ContentTextOrigin
    text_origin_ref: str | None
    title: str | None
    body: str | None
    truncation_reason: ContentTruncationReason | None
    relations: list[ContentVersionRelationView]


class ContentVisibilityView(OutputModel):
    id: UUID
    observed_at: datetime
    received_at: datetime
    status: ContentVisibilityStatus
    basis: ContentVisibilityBasis


class ContentVersionHistoryView(OutputModel):
    content_version: ContentVersionView
    first_observed_at: datetime
    last_observed_at: datetime
    observation_count: int = Field(ge=1)


class ContentObservationView(OutputModel):
    id: UUID
    observed_at: datetime
    received_at: datetime
    published_at: datetime | None
    published_at_fractional_digits: int | None = Field(ge=0, le=6)
    canonical_url: str | None
    final_url: str | None
    author_external_id: str | None
    metrics: ContentMetricView
    content_version: ContentVersionView | None


class ContentDiscoveryView(OutputModel):
    job_id: UUID
    configuration_ref: str
    configuration_version: int = Field(ge=1)
    first_observed_at: datetime
    scan_kind: CollectionScanKind | None


class ContentRecordSummaryView(OutputModel):
    id: UUID
    source_key: str
    object_type: Literal["post", "comment", "webpage"]
    native_scope: str | None
    external_id: str
    latest_observation: ContentObservationView
    current_visibility: ContentVisibilityView | None
    discovery_count: int = Field(ge=1)


class ContentRecordDetailView(ContentRecordSummaryView):
    discoveries: list[ContentDiscoveryView]
    version_history: list[ContentVersionHistoryView]
    visibility_history: list[ContentVisibilityView]
