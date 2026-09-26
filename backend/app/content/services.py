from __future__ import annotations

import hashlib
import json
import re
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from enum import StrEnum
from urllib.parse import urlsplit
from uuid import UUID, uuid4, uuid5

import structlog
from sqlalchemy import and_, case, func, or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from connections.schemas import (
    ConnectionEvidenceOutcome,
    PersistedReadEvidenceInput,
    SourceEntryPoint,
)
from connections.services import (
    AppliedSourcePreset,
    SourceCapabilityEvidenceService,
    load_applied_source_presets_in_transaction,
)
from content.models import (
    ContentDiscovery,
    ContentObservation,
    ContentRecord,
    ContentThread,
    ContentVersion,
    ContentVersionRelation,
    ContentVisibilityObservation,
)
from content.schemas import (
    AnalysisCommentContentView,
    AnalysisPostContentView,
    CollectionContentCountView,
    CommentCollectionRunInput,
    ContentDiscoveryView,
    ContentMetricView,
    ContentObservationView,
    ContentRecordDetailView,
    ContentRecordSummaryView,
    ContentRelationType,
    ContentTextOrigin,
    ContentTextScope,
    ContentTruncationReason,
    ContentVersionHistoryView,
    ContentVersionRelationView,
    ContentVersionView,
    ContentVisibilityBasis,
    ContentVisibilityStatus,
    ContentVisibilityView,
    PersistContentDocumentInput,
    PersistContentPostInput,
    RecordContentVisibilityInput,
)
from core.errors import ApplicationError
from evidence.schemas import CleanupTargetKind, CleanupTargetSpec
from evidence.services import LifecycleService, load_readable_resource_ids
from jobs.services import (
    ContentJobContext,
    JobService,
    RecentCommentJobTarget,
    load_content_job_context,
    load_content_job_contexts,
    load_recent_comment_job_targets_in_transaction,
)
from monitors.services import (
    ActiveTopicScan,
    MonitorScheduleService,
    evaluate_monitor_rules,
)
from sources.adapters.web_targets import normalize_web_url
from sources.contracts import SourceCapability

type Clock = Callable[[], datetime]

_RESOURCE_TYPE = "content_observation"
_ALLOWED_FIELDS = frozenset(
    {
        "object_type",
        "external_id",
        "canonical_url",
        "author_external_id",
        "author_name",
        "post_external_id",
        "parent_comment_external_id",
        "published_at",
        "like_count",
        "comment_count",
        "repost_count",
        "view_count",
        "play_count",
        "danmaku_count",
        "text_scope",
        "text_origin",
        "text_origin_ref",
        "title",
        "body",
        "truncation_reason",
        "quote_target_external_id",
        "quote_target_native_scope",
        "quote_target_author_external_id",
        "repost_target_external_id",
        "repost_target_native_scope",
        "repost_target_author_external_id",
    }
)
_DOCUMENT_ALLOWED_FIELDS = frozenset(
    {
        "object_type",
        "request_url",
        "final_url",
        "published_at",
        "text_scope",
        "text_origin",
        "text_origin_ref",
        "title",
        "body",
        "truncation_reason",
    }
)
_METRIC_FIELDS = (
    "like_count",
    "comment_count",
    "repost_count",
    "view_count",
    "play_count",
    "danmaku_count",
)
_MAX_BIGINT = 9_223_372_036_854_775_807
COMMENT_OPERATION_NAMESPACE = UUID("755fda03-92d0-4fa9-afc2-cfacb6d5d0a8")
_COMMENT_REFRESH_INTERVAL = timedelta(hours=6)
_COMMENT_POST_LIFETIME = timedelta(hours=24)
_COMMENT_TOPIC_LIMIT = 20
_RFC3339_PATTERN = re.compile(
    r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.(\d{1,6}))?(?:Z|[+-]\d{2}:\d{2})$"
)
_VERSION_FIELD_NAMES = frozenset(
    {
        "text_scope",
        "text_origin",
        "text_origin_ref",
        "title",
        "body",
        "truncation_reason",
        "quote_target_external_id",
        "quote_target_native_scope",
        "quote_target_author_external_id",
        "repost_target_external_id",
        "repost_target_native_scope",
        "repost_target_author_external_id",
    }
)


@dataclass(frozen=True)
class _ContentRelationValues:
    relation_type: ContentRelationType
    target_native_scope: str | None
    target_external_id: str
    target_author_external_id: str | None

    def canonical_value(self) -> dict[str, str | None]:
        return {
            "relation_type": self.relation_type.value,
            "target_native_scope": self.target_native_scope,
            "target_external_id": self.target_external_id,
            "target_author_external_id": self.target_author_external_id,
        }


@dataclass(frozen=True)
class _ContentVersionValues:
    fingerprint: bytes
    text_scope: ContentTextScope
    text_origin: ContentTextOrigin
    text_origin_ref: str | None
    title: str | None
    body: str | None
    truncation_reason: ContentTruncationReason | None
    relations: tuple[_ContentRelationValues, ...]


def _optional_identifier(fields: Mapping[str, object], name: str) -> str | None:
    value = fields.get(name)
    if value is None:
        return None
    if (
        not isinstance(value, str)
        or not value
        or value != value.strip()
        or any(ord(character) < 32 or ord(character) == 127 for character in value)
    ):
        raise ValueError(f"{name} must be an opaque non-empty identifier")
    if len(value) > 512:
        raise ValueError(f"{name} is too long")
    return value


def _optional_url(fields: Mapping[str, object], name: str = "canonical_url") -> str | None:
    value = fields.get(name)
    if value is None:
        return None
    if not isinstance(value, str) or not value or value != value.strip() or len(value) > 2048:
        raise ValueError(f"{name} must be a bounded URL")
    if any(ord(character) < 32 or ord(character) == 127 for character in value):
        raise ValueError(f"{name} cannot contain controls")
    parsed = urlsplit(value)
    if (
        parsed.scheme not in {"http", "https"}
        or parsed.hostname is None
        or parsed.username is not None
        or parsed.password is not None
    ):
        raise ValueError(f"{name} must be an http or https URL without credentials")
    return value


def _normalized_web_url(fields: Mapping[str, object], name: str) -> str:
    value = _optional_url(fields, name)
    if value is None:
        raise ValueError(f"{name} is required")
    hostname = urlsplit(value).hostname
    assert hostname is not None
    try:
        normalized = normalize_web_url(value, allowed_hosts=frozenset({hostname}))
    except ValueError as error:
        raise ValueError(f"{name} must be a normalized public web URL") from error
    if normalized != value:
        raise ValueError(f"{name} must be normalized")
    return normalized


def _optional_datetime_with_precision(
    fields: Mapping[str, object], name: str
) -> tuple[datetime | None, int | None]:
    value = fields.get(name)
    if value is None:
        return None, None
    if not isinstance(value, str):
        raise ValueError(f"{name} must be an RFC 3339 string")
    match = _RFC3339_PATTERN.fullmatch(value)
    if match is None:
        raise ValueError(f"{name} must be an RFC 3339 string with at most 6 fractional digits")
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise ValueError(f"{name} must be an RFC 3339 string") from error
    if parsed.utcoffset() is None:
        raise ValueError(f"{name} must be timezone-aware")
    fractional = match.group(1)
    return parsed, len(fractional) if fractional is not None else 0


def _optional_metric(fields: Mapping[str, object], name: str) -> int | None:
    value = fields.get(name)
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, int) or not 0 <= value <= _MAX_BIGINT:
        raise ValueError(f"{name} must be a non-negative integer or null")
    return value


def _optional_text(
    fields: Mapping[str, object],
    name: str,
    *,
    max_length: int,
) -> str | None:
    value = fields.get(name)
    if value is None:
        return None
    if not isinstance(value, str) or not value.strip() or len(value) > max_length:
        raise ValueError(f"{name} must be non-empty and at most {max_length} characters")
    if any(ord(character) < 32 and character not in "\t\n\r" for character in value) or any(
        ord(character) == 127 for character in value
    ):
        raise ValueError(f"{name} contains unsupported control characters")
    return value


def _required_enum[EnumT: StrEnum](
    fields: Mapping[str, object],
    name: str,
    enum_type: type[EnumT],
) -> EnumT:
    value = fields.get(name)
    if not isinstance(value, str):
        raise ValueError(f"{name} is required")
    try:
        return enum_type(value)
    except ValueError as error:
        raise ValueError(f"{name} has an unsupported value") from error


def _optional_truncation_reason(
    fields: Mapping[str, object],
) -> ContentTruncationReason | None:
    value = fields.get("truncation_reason")
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError("truncation_reason has an unsupported value")
    try:
        return ContentTruncationReason(value)
    except ValueError as error:
        raise ValueError("truncation_reason has an unsupported value") from error


def _relation_values(
    fields: Mapping[str, object],
    relation_type: ContentRelationType,
) -> _ContentRelationValues | None:
    prefix = relation_type.value
    external_id = _optional_identifier(fields, f"{prefix}_target_external_id")
    native_scope = _optional_identifier(fields, f"{prefix}_target_native_scope")
    author_external_id = _optional_identifier(fields, f"{prefix}_target_author_external_id")
    if external_id is None:
        if native_scope is not None or author_external_id is not None:
            raise ValueError(f"{prefix} target identity requires an external_id")
        return None
    return _ContentRelationValues(
        relation_type=relation_type,
        target_native_scope=native_scope,
        target_external_id=external_id,
        target_author_external_id=author_external_id,
    )


def _content_version_values(fields: Mapping[str, object]) -> _ContentVersionValues | None:
    if not any(fields.get(name) is not None for name in _VERSION_FIELD_NAMES):
        return None
    text_scope = _required_enum(fields, "text_scope", ContentTextScope)
    text_origin = _required_enum(fields, "text_origin", ContentTextOrigin)
    text_origin_ref = _optional_identifier(fields, "text_origin_ref")
    title = _optional_text(fields, "title", max_length=2_000)
    body = _optional_text(fields, "body", max_length=100_000)
    truncation_reason = _optional_truncation_reason(fields)
    relations = tuple(
        relation
        for relation_type in (ContentRelationType.QUOTE, ContentRelationType.REPOST)
        if (relation := _relation_values(fields, relation_type)) is not None
    )

    if text_origin is ContentTextOrigin.SOURCE and text_origin_ref is not None:
        raise ValueError("source text cannot declare a machine extraction reference")
    if text_origin is ContentTextOrigin.MACHINE_EXTRACTED and text_origin_ref is None:
        raise ValueError("machine-extracted text requires text_origin_ref")
    if text_scope is ContentTextScope.MEDIA_ONLY:
        if title is not None or body is not None:
            raise ValueError("media-only content cannot contain invented title or body text")
        if text_origin is not ContentTextOrigin.SOURCE:
            raise ValueError("media-only content must describe the source observation")
    elif title is None and body is None:
        raise ValueError("text content requires a title or body")
    if text_scope is ContentTextScope.TRUNCATED:
        if truncation_reason is None:
            raise ValueError("truncated text requires truncation_reason")
    elif truncation_reason is not None:
        raise ValueError("truncation_reason is only valid for truncated text")

    canonical = {
        "text_scope": text_scope.value,
        "text_origin": text_origin.value,
        "text_origin_ref": text_origin_ref,
        "title": title,
        "body": body,
        "truncation_reason": truncation_reason.value if truncation_reason else None,
        "relations": [relation.canonical_value() for relation in relations],
    }
    fingerprint = hashlib.sha256(
        json.dumps(canonical, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    ).digest()
    return _ContentVersionValues(
        fingerprint=fingerprint,
        text_scope=text_scope,
        text_origin=text_origin,
        text_origin_ref=text_origin_ref,
        title=title,
        body=body,
        truncation_reason=truncation_reason,
        relations=relations,
    )


class ContentService:
    def __init__(self, session: Session, *, clock: Clock | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def collection_counts_in_transaction(
        self, *, owner_id: UUID, job_ids: tuple[UUID, ...]
    ) -> tuple[CollectionContentCountView, ...]:
        """Count committed observations and first ingestion by stable source identity."""
        if not self._session.in_transaction():
            raise RuntimeError("collection count reads require the caller's transaction")
        if not job_ids:
            return ()
        observations = self._session.scalars(
            select(ContentObservation).where(
                ContentObservation.owner_id == owner_id,
                ContentObservation.job_id.in_(job_ids),
            )
        ).all()
        candidate_ids = {item.content_id for item in observations}
        first_by_content: dict[UUID, ContentObservation] = {}
        if candidate_ids:
            history = self._session.scalars(
                select(ContentObservation).where(
                    ContentObservation.owner_id == owner_id,
                    ContentObservation.content_id.in_(candidate_ids),
                )
            ).all()
            for item in history:
                previous = first_by_content.get(item.content_id)
                if previous is None or (item.received_at, item.id) < (
                    previous.received_at,
                    previous.id,
                ):
                    first_by_content[item.content_id] = item
        return tuple(
            CollectionContentCountView(
                job_id=job_id,
                observation_count=sum(item.job_id == job_id for item in observations),
                ingested_count=len(
                    {item.content_id for item in observations if item.job_id == job_id}
                ),
                first_ingested_count=sum(
                    item.job_id == job_id for item in first_by_content.values()
                ),
                deduplicated_count=sum(item.job_id == job_id for item in observations)
                - sum(item.job_id == job_id for item in first_by_content.values()),
                content_version_ids=tuple(
                    sorted(
                        {
                            item.content_version_id
                            for item in observations
                            if item.job_id == job_id and item.content_version_id is not None
                        },
                        key=str,
                    )
                ),
            )
            for job_id in job_ids
        )

    def require_persisted_document_result(
        self,
        *,
        owner_id: UUID,
        job_id: UUID,
        content_id: UUID,
        observation_id: UUID,
    ) -> None:
        """Verify a durable webpage checkpoint before completing recovered work."""
        self._session.rollback()
        with self._session.begin():
            observation = self._session.scalar(
                select(ContentObservation)
                .join(ContentRecord, ContentRecord.id == ContentObservation.content_id)
                .where(
                    ContentObservation.owner_id == owner_id,
                    ContentObservation.id == observation_id,
                    ContentObservation.content_id == content_id,
                    ContentObservation.job_id == job_id,
                    ContentRecord.owner_id == owner_id,
                    ContentRecord.object_type == "webpage",
                )
            )
            discovery = self._session.scalar(
                select(ContentDiscovery.id).where(
                    ContentDiscovery.owner_id == owner_id,
                    ContentDiscovery.content_id == content_id,
                    ContentDiscovery.job_id == job_id,
                )
            )
            if observation is None or discovery is None:
                raise ApplicationError("resource_not_found")

    @staticmethod
    def _selected_version_view(
        observation: ContentObservation,
        version_views: Mapping[UUID, ContentVersionView],
    ) -> ContentVersionView | None:
        if observation.content_version_id is None:
            return None
        return version_views.get(observation.content_version_id)

    def persist_post(
        self,
        *,
        owner_id: UUID,
        command: PersistContentPostInput,
    ) -> ContentRecordDetailView:
        self._session.rollback()
        with self._session.begin():
            return self.persist_post_in_transaction(owner_id=owner_id, command=command)

    def persist_post_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: PersistContentPostInput,
    ) -> ContentRecordDetailView:
        """Persist an admitted social post within the caller's page transaction."""
        now = self._clock()
        fields, external_id = self._admitted_social_fields(
            owner_id=owner_id, command=command, now=now, object_type="post"
        )
        if "post_external_id" in fields or "parent_comment_external_id" in fields:
            raise ValueError("posts cannot declare comment thread fields")
        return self._persist_admitted_content_in_transaction(
            owner_id=owner_id,
            command=command,
            object_type="post",
            native_scope=command.native_scope,
            external_id=external_id,
            observation_values=self._observation_values(fields),
            version_values=_content_version_values(fields),
            now=now,
        )

    def persist_comment(
        self,
        *,
        owner_id: UUID,
        command: PersistContentPostInput,
    ) -> ContentRecordDetailView:
        self._session.rollback()
        with self._session.begin():
            return self.persist_comment_in_transaction(owner_id=owner_id, command=command)

    def persist_comment_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: PersistContentPostInput,
    ) -> ContentRecordDetailView:
        """Persist an admitted comment and link it to its post and parent comment.

        The post and the parent comment may arrive later than the comment, so their
        records are created as identity-only placeholders when missing.
        """
        now = self._clock()
        fields, external_id = self._admitted_social_fields(
            owner_id=owner_id, command=command, now=now, object_type="comment"
        )
        post_external_id = _optional_identifier(fields, "post_external_id")
        if post_external_id is None:
            raise ValueError("post_external_id is required for comments")
        parent_external_id = _optional_identifier(fields, "parent_comment_external_id")
        if parent_external_id == external_id:
            raise ValueError("a comment cannot be its own parent")
        observation_fields = {
            name: value
            for name, value in fields.items()
            if name not in {"post_external_id", "parent_comment_external_id"}
        }
        saved = self._persist_admitted_content_in_transaction(
            owner_id=owner_id,
            command=command,
            object_type="comment",
            native_scope=command.native_scope,
            external_id=external_id,
            observation_values=self._observation_values(observation_fields),
            version_values=_content_version_values(observation_fields),
            now=now,
        )
        source_key = command.admission.source_key
        post = self._find_or_create_content(
            owner_id=owner_id,
            source_key=source_key,
            object_type="post",
            native_scope=command.native_scope,
            external_id=post_external_id,
            created_at=now,
        )
        parent = (
            None
            if parent_external_id is None
            else self._find_or_create_content(
                owner_id=owner_id,
                source_key=source_key,
                object_type="comment",
                native_scope=command.native_scope,
                external_id=parent_external_id,
                created_at=now,
            )
        )
        parent_id = parent.id if parent is not None else None
        inserted = self._session.scalar(
            insert(ContentThread)
            .values(
                owner_id=owner_id,
                content_id=saved.id,
                post_content_id=post.id,
                parent_content_id=parent_id,
                created_at=now,
            )
            .on_conflict_do_nothing(index_elements=["owner_id", "content_id"])
            .returning(ContentThread.content_id)
        )
        if inserted is None:
            existing = self._session.get(ContentThread, (owner_id, saved.id))
            if (
                existing is None
                or existing.post_content_id != post.id
                or existing.parent_content_id != parent_id
            ):
                raise ValueError("comment thread conflicts with the stored thread")
        return saved

    def _admitted_social_fields(
        self,
        *,
        owner_id: UUID,
        command: PersistContentPostInput,
        now: datetime,
        object_type: str,
    ) -> tuple[dict[str, object], str]:
        if now.utcoffset() is None:
            raise ValueError("clock must return a timezone-aware datetime")
        if command.admission.owner_id != owner_id:
            raise ApplicationError("resource_not_found")
        if command.admission.collected_at > now:
            raise ValueError("collected_at cannot be in the future")
        fields: dict[str, object] = dict(command.admission.fields)
        unknown_fields = set(fields) - _ALLOWED_FIELDS
        if unknown_fields:
            raise ValueError("admitted payload contains fields outside the S01 contract")
        if fields.get("object_type", "post") != object_type:
            raise ValueError(f"expected a {object_type} object")
        external_id = _optional_identifier(fields, "external_id")
        if external_id is None:
            raise ValueError("external_id is required")
        return fields, external_id

    def persist_document_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: PersistContentDocumentInput,
    ) -> ContentRecordDetailView:
        """Persist one normalized document inside an existing outer transaction."""
        now = self._clock()
        if now.utcoffset() is None:
            raise ValueError("clock must return a timezone-aware datetime")
        if command.admission.owner_id != owner_id:
            raise ApplicationError("resource_not_found")
        if command.admission.collected_at > now:
            raise ValueError("collected_at cannot be in the future")
        fields = dict(command.admission.fields)
        if set(fields) - _DOCUMENT_ALLOWED_FIELDS:
            raise ValueError("admitted payload contains fields outside the webpage contract")
        if fields.get("object_type") != "webpage":
            raise ValueError("document persistence only accepts webpage objects")
        if fields.get("text_origin") != ContentTextOrigin.MACHINE_EXTRACTED.value:
            raise ValueError("webpage text must be machine-extracted")
        text_scope = fields.get("text_scope")
        if text_scope not in {ContentTextScope.FULL.value, ContentTextScope.TRUNCATED.value}:
            raise ValueError("webpage text_scope must be full or truncated")
        if not isinstance(fields.get("body"), str):
            raise ValueError("webpage body is required")
        if (
            text_scope == ContentTextScope.TRUNCATED.value
            and fields.get("truncation_reason") != ContentTruncationReason.COLLECTOR_LIMIT.value
        ):
            raise ValueError("truncated webpage text requires collector_limit")

        request_url = _normalized_web_url(fields, "request_url")
        final_url = _normalized_web_url(fields, "final_url")
        native_scope = urlsplit(request_url).hostname
        assert native_scope is not None
        external_id = hashlib.sha256(request_url.encode()).hexdigest()
        persistence_fields = {
            name: value
            for name, value in fields.items()
            if name not in {"object_type", "request_url", "final_url"}
        }
        persistence_fields["canonical_url"] = request_url
        persistence_fields["final_url"] = final_url
        observation_values = self._observation_values(persistence_fields)
        version_values = _content_version_values(persistence_fields)
        if version_values is None:
            raise ValueError("webpage content version is required")
        return self._persist_admitted_content_in_transaction(
            owner_id=owner_id,
            command=command,
            object_type="webpage",
            native_scope=native_scope,
            external_id=external_id,
            observation_values=observation_values,
            version_values=version_values,
            now=now,
        )

    def _persist_admitted_content_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: PersistContentPostInput | PersistContentDocumentInput,
        object_type: str,
        native_scope: str | None,
        external_id: str,
        observation_values: dict[str, object],
        version_values: _ContentVersionValues | None,
        now: datetime,
    ) -> ContentRecordDetailView:
        job = load_content_job_context(
            self._session,
            owner_id=owner_id,
            job_id=command.job_id,
        )
        if (
            job is None
            or job.source_key != command.admission.source_key
            or job.source_capability != command.admission.capability
        ):
            raise ApplicationError("resource_not_found")
        content = self._find_or_create_content(
            owner_id=owner_id,
            source_key=command.admission.source_key,
            object_type=object_type,
            native_scope=native_scope,
            external_id=external_id,
            created_at=now,
        )
        content_version = self._find_or_create_content_version(
            owner_id=owner_id,
            content_id=content.id,
            values=version_values,
            created_at=now,
        )
        observation_values["content_version_id"] = content_version.id if content_version else None
        observation = self._find_or_create_observation(
            owner_id=owner_id,
            content_id=content.id,
            job_id=job.job_id,
            source_operation_id=command.source_operation_id,
            observed_at=command.admission.collected_at,
            received_at=now,
            values=observation_values,
        )
        self._find_or_create_visibility_observation(
            owner_id=owner_id,
            content_id=content.id,
            job_id=job.job_id,
            source_operation_id=command.source_operation_id,
            observed_at=observation.observed_at,
            received_at=observation.received_at,
            status=ContentVisibilityStatus.VISIBLE,
            basis=ContentVisibilityBasis.CONTENT_RETURNED,
        )
        self._find_or_create_discovery(
            owner_id=owner_id,
            content_id=content.id,
            job_id=job.job_id,
            first_observed_at=observation.observed_at,
            created_at=now,
        )
        LifecycleService(self._session, clock=self._clock).track_resource_in_transaction(
            owner_id=owner_id,
            resource_type=_RESOURCE_TYPE,
            resource_id=observation.id,
            admission=command.admission,
            cleanup_targets=[
                CleanupTargetSpec(
                    kind=CleanupTargetKind.POSTGRES_CONTENT_OBSERVATION,
                    reference=str(observation.id),
                )
            ],
        )
        SourceCapabilityEvidenceService(
            self._session,
            clock=self._clock,
        ).record_persisted_read_in_transaction(
            owner_id=owner_id,
            command=PersistedReadEvidenceInput(
                operation_id=command.source_operation_id,
                connection_id=command.connection_id,
                connection_version=command.connection_version,
                capability=command.admission.capability,
                entry_point=command.entry_point,
                outcome=ConnectionEvidenceOutcome.SUCCEEDED,
                stop_reason=None,
                resource_ref=f"content_observation:{observation.id}",
                component_name=command.component_name,
                component_version=command.component_version,
            ),
        )
        readable_observations = self._readable_observation_history(
            owner_id=owner_id,
            content_id=content.id,
            now=now,
        )
        version_views = self._content_version_views(
            owner_id=owner_id,
            content_observations=[(content, item) for item in readable_observations],
            now=now,
        )
        visibility_history = self._visibility_history(
            owner_id=owner_id,
            content_id=content.id,
        )
        return self._detail_view(
            content=content,
            observation=observation,
            content_version=self._selected_version_view(observation, version_views),
            current_visibility=(visibility_history[0] if visibility_history else None),
            discoveries=self._discoveries(owner_id, content.id, {job.job_id}),
            job_contexts={job.job_id: job},
            version_history=self._version_history(readable_observations, version_views),
            visibility_history=visibility_history,
        )

    def record_visibility(
        self,
        *,
        owner_id: UUID,
        command: RecordContentVisibilityInput,
    ) -> ContentVisibilityView:
        now = self._clock()
        if now.utcoffset() is None:
            raise ValueError("clock must return a timezone-aware datetime")
        if command.observed_at > now:
            raise ValueError("observed_at cannot be in the future")
        self._session.rollback()
        with self._session.begin():
            content = self._session.scalar(
                select(ContentRecord)
                .where(
                    ContentRecord.owner_id == owner_id,
                    ContentRecord.id == command.content_id,
                )
                .with_for_update()
            )
            job = load_content_job_context(
                self._session,
                owner_id=owner_id,
                job_id=command.job_id,
            )
            if content is None or job is None or job.source_key != content.source_key:
                raise ApplicationError("resource_not_found")
            visibility = self._find_or_create_visibility_observation(
                owner_id=owner_id,
                content_id=content.id,
                job_id=job.job_id,
                source_operation_id=command.source_operation_id,
                observed_at=command.observed_at,
                received_at=now,
                status=command.status,
                basis=command.basis,
            )
            return self._visibility_view(visibility)

    def get_content(self, *, owner_id: UUID, content_id: UUID) -> ContentRecordDetailView:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            content = self._session.scalar(
                select(ContentRecord).where(
                    ContentRecord.owner_id == owner_id,
                    ContentRecord.id == content_id,
                )
            )
            if content is None:
                raise ApplicationError("resource_not_found")
            projected = self._readable_observations(
                owner_id=owner_id,
                content_ids={content.id},
                now=now,
            ).get(content.id)
            if projected is None:
                raise ApplicationError("resource_not_found")
            observation, readable_job_ids = projected
            discoveries = self._discoveries(owner_id, content.id, readable_job_ids)
            contexts = load_content_job_contexts(
                self._session,
                owner_id=owner_id,
                job_ids={item.job_id for item in discoveries},
            )
            readable_observations = self._readable_observation_history(
                owner_id=owner_id,
                content_id=content.id,
                now=now,
            )
            version_views = self._content_version_views(
                owner_id=owner_id,
                content_observations=[(content, item) for item in readable_observations],
                now=now,
            )
            visibility_history = self._visibility_history(
                owner_id=owner_id,
                content_id=content.id,
            )
            return self._detail_view(
                content=content,
                observation=observation,
                content_version=self._selected_version_view(observation, version_views),
                current_visibility=(visibility_history[0] if visibility_history else None),
                discoveries=discoveries,
                job_contexts=contexts,
                version_history=self._version_history(readable_observations, version_views),
                visibility_history=visibility_history,
            )

    def list_contents(
        self,
        *,
        owner_id: UUID,
        cursor: UUID | None,
        limit: int,
    ) -> tuple[list[ContentRecordSummaryView], str | None]:
        if not 1 <= limit <= 100:
            raise ValueError("limit must be between 1 and 100")
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            visible: list[tuple[ContentRecord, ContentObservation, set[UUID]]] = []
            scan_cursor = cursor
            batch_size = max(50, limit * 2)
            while len(visible) <= limit:
                statement = (
                    select(ContentRecord)
                    .where(ContentRecord.owner_id == owner_id)
                    .order_by(ContentRecord.id)
                    .limit(batch_size)
                )
                if scan_cursor is not None:
                    statement = statement.where(ContentRecord.id > scan_cursor)
                records = list(self._session.scalars(statement).all())
                if not records:
                    break
                scan_cursor = records[-1].id
                projections = self._readable_observations(
                    owner_id=owner_id,
                    content_ids={record.id for record in records},
                    now=now,
                )
                visible.extend(
                    (record, projections[record.id][0], projections[record.id][1])
                    for record in records
                    if record.id in projections
                )
                if len(records) < batch_size:
                    break
            has_more = len(visible) > limit
            page = visible[:limit]
            discovery_counts = self._discovery_counts(
                owner_id=owner_id,
                readable_jobs={record.id: job_ids for record, _, job_ids in page},
            )
            version_views = self._content_version_views(
                owner_id=owner_id,
                content_observations=[(content, observation) for content, observation, _ in page],
                now=now,
            )
            current_visibility = self._current_visibility_views(
                owner_id=owner_id,
                content_ids={content.id for content, _, _ in page},
            )
            items = [
                self._summary_view(
                    content=content,
                    observation=observation,
                    content_version=self._selected_version_view(observation, version_views),
                    current_visibility=current_visibility.get(content.id),
                    discovery_count=discovery_counts.get(content.id, 0),
                )
                for content, observation, _ in page
            ]
            next_cursor = str(page[-1][0].id) if has_more else None
        return items, next_cursor

    @staticmethod
    def _observation_values(fields: Mapping[str, object]) -> dict[str, object]:
        published_at, published_at_fractional_digits = _optional_datetime_with_precision(
            fields, "published_at"
        )
        return {
            "canonical_url": _optional_url(fields),
            "final_url": _optional_url(fields, "final_url"),
            "author_external_id": _optional_identifier(fields, "author_external_id"),
            "author_name": _optional_text(fields, "author_name", max_length=256),
            "published_at": published_at,
            "published_at_fractional_digits": published_at_fractional_digits,
            **{name: _optional_metric(fields, name) for name in _METRIC_FIELDS},
        }

    def _find_or_create_content(
        self,
        *,
        owner_id: UUID,
        source_key: str,
        object_type: str,
        native_scope: str | None,
        external_id: str,
        created_at: datetime,
    ) -> ContentRecord:
        content_id = uuid4()
        inserted_id = self._session.scalar(
            insert(ContentRecord)
            .values(
                id=content_id,
                owner_id=owner_id,
                source_key=source_key,
                object_type=object_type,
                native_scope=native_scope,
                external_id=external_id,
                created_at=created_at,
            )
            .on_conflict_do_nothing(constraint="content_records_source_identity_key")
            .returning(ContentRecord.id)
        )
        if inserted_id is not None:
            return ContentRecord(
                id=inserted_id,
                owner_id=owner_id,
                source_key=source_key,
                object_type=object_type,
                native_scope=native_scope,
                external_id=external_id,
                created_at=created_at,
            )
        conditions = [
            ContentRecord.owner_id == owner_id,
            ContentRecord.source_key == source_key,
            ContentRecord.object_type == object_type,
            ContentRecord.external_id == external_id,
        ]
        conditions.append(
            ContentRecord.native_scope.is_(None)
            if native_scope is None
            else ContentRecord.native_scope == native_scope
        )
        existing = self._session.scalar(select(ContentRecord).where(*conditions).with_for_update())
        if existing is None:
            raise RuntimeError("conflicting content identity is not visible")
        return existing

    def _find_or_create_content_version(
        self,
        *,
        owner_id: UUID,
        content_id: UUID,
        values: _ContentVersionValues | None,
        created_at: datetime,
    ) -> ContentVersion | None:
        if values is None:
            return None
        version_id = uuid4()
        inserted_id = self._session.scalar(
            insert(ContentVersion)
            .values(
                id=version_id,
                owner_id=owner_id,
                content_id=content_id,
                fingerprint=values.fingerprint,
                text_scope=values.text_scope.value,
                text_origin=values.text_origin.value,
                text_origin_ref=values.text_origin_ref,
                title=values.title,
                body=values.body,
                truncation_reason=(
                    values.truncation_reason.value if values.truncation_reason else None
                ),
                created_at=created_at,
            )
            .on_conflict_do_nothing(constraint="content_versions_owner_content_fingerprint_key")
            .returning(ContentVersion.id)
        )
        if inserted_id is None:
            existing = self._session.scalar(
                select(ContentVersion)
                .where(
                    ContentVersion.owner_id == owner_id,
                    ContentVersion.content_id == content_id,
                    ContentVersion.fingerprint == values.fingerprint,
                )
                .with_for_update()
            )
            if existing is None:
                raise RuntimeError("conflicting content version is not visible")
            return existing

        for relation in values.relations:
            self._session.add(
                ContentVersionRelation(
                    id=uuid4(),
                    owner_id=owner_id,
                    content_version_id=inserted_id,
                    relation_type=relation.relation_type.value,
                    target_native_scope=relation.target_native_scope,
                    target_external_id=relation.target_external_id,
                    target_author_external_id=relation.target_author_external_id,
                )
            )
        self._session.flush()
        return ContentVersion(
            id=inserted_id,
            owner_id=owner_id,
            content_id=content_id,
            fingerprint=values.fingerprint,
            text_scope=values.text_scope.value,
            text_origin=values.text_origin.value,
            text_origin_ref=values.text_origin_ref,
            title=values.title,
            body=values.body,
            truncation_reason=(
                values.truncation_reason.value if values.truncation_reason else None
            ),
            created_at=created_at,
        )

    def _find_or_create_observation(
        self,
        *,
        owner_id: UUID,
        content_id: UUID,
        job_id: UUID,
        source_operation_id: UUID,
        observed_at: datetime,
        received_at: datetime,
        values: Mapping[str, object],
    ) -> ContentObservation:
        observation_id = uuid4()
        inserted_id = self._session.scalar(
            insert(ContentObservation)
            .values(
                id=observation_id,
                owner_id=owner_id,
                content_id=content_id,
                job_id=job_id,
                source_operation_id=source_operation_id,
                observed_at=observed_at,
                received_at=received_at,
                **values,
            )
            .on_conflict_do_nothing(constraint="content_observations_owner_content_operation_key")
            .returning(ContentObservation.id)
        )
        if inserted_id is not None:
            return ContentObservation(
                id=inserted_id,
                owner_id=owner_id,
                content_id=content_id,
                job_id=job_id,
                source_operation_id=source_operation_id,
                observed_at=observed_at,
                received_at=received_at,
                **values,
            )
        existing = self._session.scalar(
            select(ContentObservation)
            .where(
                ContentObservation.owner_id == owner_id,
                ContentObservation.content_id == content_id,
                ContentObservation.source_operation_id == source_operation_id,
            )
            .with_for_update()
        )
        if existing is None:
            raise RuntimeError("conflicting content observation is not visible")
        if not self._observation_matches(
            existing,
            job_id=job_id,
            observed_at=observed_at,
            values=values,
        ):
            raise ApplicationError("idempotency_conflict")
        return existing

    def _find_or_create_visibility_observation(
        self,
        *,
        owner_id: UUID,
        content_id: UUID,
        job_id: UUID,
        source_operation_id: UUID,
        observed_at: datetime,
        received_at: datetime,
        status: ContentVisibilityStatus,
        basis: ContentVisibilityBasis,
    ) -> ContentVisibilityObservation:
        visibility_id = uuid4()
        inserted_id = self._session.scalar(
            insert(ContentVisibilityObservation)
            .values(
                id=visibility_id,
                owner_id=owner_id,
                content_id=content_id,
                job_id=job_id,
                source_operation_id=source_operation_id,
                observed_at=observed_at,
                received_at=received_at,
                status=status.value,
                basis=basis.value,
            )
            .on_conflict_do_nothing(
                constraint="content_visibility_observations_owner_content_operation_key"
            )
            .returning(ContentVisibilityObservation.id)
        )
        if inserted_id is not None:
            return ContentVisibilityObservation(
                id=inserted_id,
                owner_id=owner_id,
                content_id=content_id,
                job_id=job_id,
                source_operation_id=source_operation_id,
                observed_at=observed_at,
                received_at=received_at,
                status=status.value,
                basis=basis.value,
            )
        existing = self._session.scalar(
            select(ContentVisibilityObservation)
            .where(
                ContentVisibilityObservation.owner_id == owner_id,
                ContentVisibilityObservation.content_id == content_id,
                ContentVisibilityObservation.source_operation_id == source_operation_id,
            )
            .with_for_update()
        )
        if existing is None:
            raise RuntimeError("conflicting content visibility observation is not visible")
        if (
            existing.job_id != job_id
            or existing.observed_at != observed_at
            or existing.status != status.value
            or existing.basis != basis.value
        ):
            raise ApplicationError("idempotency_conflict")
        return existing

    def _find_or_create_discovery(
        self,
        *,
        owner_id: UUID,
        content_id: UUID,
        job_id: UUID,
        first_observed_at: datetime,
        created_at: datetime,
    ) -> None:
        self._session.execute(
            insert(ContentDiscovery)
            .values(
                id=uuid4(),
                owner_id=owner_id,
                content_id=content_id,
                job_id=job_id,
                first_observed_at=first_observed_at,
                created_at=created_at,
            )
            .on_conflict_do_nothing(constraint="content_discoveries_owner_content_job_key")
        )

    @staticmethod
    def _observation_matches(
        observation: ContentObservation,
        *,
        job_id: UUID,
        observed_at: datetime,
        values: Mapping[str, object],
    ) -> bool:
        return (
            observation.job_id == job_id
            and observation.observed_at == observed_at
            and all(getattr(observation, name) == value for name, value in values.items())
        )

    def _readable_observations(
        self,
        *,
        owner_id: UUID,
        content_ids: set[UUID],
        now: datetime,
    ) -> dict[UUID, tuple[ContentObservation, set[UUID]]]:
        if not content_ids:
            return {}
        observations = list(
            self._session.scalars(
                select(ContentObservation).where(
                    ContentObservation.owner_id == owner_id,
                    ContentObservation.content_id.in_(content_ids),
                )
            ).all()
        )
        readable_ids = load_readable_resource_ids(
            self._session,
            owner_id=owner_id,
            resource_type=_RESOURCE_TYPE,
            resource_ids={item.id for item in observations},
            now=now,
        )
        grouped: dict[UUID, list[ContentObservation]] = {}
        for observation in observations:
            if observation.id in readable_ids:
                grouped.setdefault(observation.content_id, []).append(observation)
        return {
            content_id: (
                max(items, key=lambda item: (item.observed_at, item.received_at, item.id)),
                {item.job_id for item in items},
            )
            for content_id, items in grouped.items()
        }

    def _readable_observation_history(
        self,
        *,
        owner_id: UUID,
        content_id: UUID,
        now: datetime,
    ) -> list[ContentObservation]:
        observations = list(
            self._session.scalars(
                select(ContentObservation).where(
                    ContentObservation.owner_id == owner_id,
                    ContentObservation.content_id == content_id,
                )
            ).all()
        )
        readable_ids = load_readable_resource_ids(
            self._session,
            owner_id=owner_id,
            resource_type=_RESOURCE_TYPE,
            resource_ids={item.id for item in observations},
            now=now,
        )
        return sorted(
            (item for item in observations if item.id in readable_ids),
            key=lambda item: (item.observed_at, item.received_at, item.id),
            reverse=True,
        )

    def _visibility_history(
        self,
        *,
        owner_id: UUID,
        content_id: UUID,
    ) -> list[ContentVisibilityView]:
        observations = self._session.scalars(
            select(ContentVisibilityObservation)
            .where(
                ContentVisibilityObservation.owner_id == owner_id,
                ContentVisibilityObservation.content_id == content_id,
            )
            .order_by(
                ContentVisibilityObservation.observed_at.desc(),
                ContentVisibilityObservation.received_at.desc(),
                ContentVisibilityObservation.id.desc(),
            )
        ).all()
        return [self._visibility_view(item) for item in observations]

    def _current_visibility_views(
        self,
        *,
        owner_id: UUID,
        content_ids: set[UUID],
    ) -> dict[UUID, ContentVisibilityView]:
        if not content_ids:
            return {}
        observations = self._session.scalars(
            select(ContentVisibilityObservation).where(
                ContentVisibilityObservation.owner_id == owner_id,
                ContentVisibilityObservation.content_id.in_(content_ids),
            )
        ).all()
        current: dict[UUID, ContentVisibilityObservation] = {}
        for item in observations:
            previous = current.get(item.content_id)
            if previous is None or (
                item.observed_at,
                item.received_at,
                item.id,
            ) > (
                previous.observed_at,
                previous.received_at,
                previous.id,
            ):
                current[item.content_id] = item
        return {content_id: self._visibility_view(item) for content_id, item in current.items()}

    def _discoveries(
        self,
        owner_id: UUID,
        content_id: UUID,
        readable_job_ids: set[UUID],
    ) -> list[ContentDiscovery]:
        if not readable_job_ids:
            return []
        return list(
            self._session.scalars(
                select(ContentDiscovery)
                .where(
                    ContentDiscovery.owner_id == owner_id,
                    ContentDiscovery.content_id == content_id,
                    ContentDiscovery.job_id.in_(readable_job_ids),
                )
                .order_by(ContentDiscovery.first_observed_at, ContentDiscovery.id)
            ).all()
        )

    def _discovery_counts(
        self,
        *,
        owner_id: UUID,
        readable_jobs: dict[UUID, set[UUID]],
    ) -> dict[UUID, int]:
        counts: dict[UUID, int] = {}
        for content_id, job_ids in readable_jobs.items():
            if not job_ids:
                continue
            count = self._session.scalar(
                select(func.count(ContentDiscovery.id)).where(
                    ContentDiscovery.owner_id == owner_id,
                    ContentDiscovery.content_id == content_id,
                    ContentDiscovery.job_id.in_(job_ids),
                )
            )
            counts[content_id] = int(count or 0)
        return counts

    def _content_version_views(
        self,
        *,
        owner_id: UUID,
        content_observations: list[tuple[ContentRecord, ContentObservation]],
        now: datetime,
    ) -> dict[UUID, ContentVersionView]:
        version_ids = {
            observation.content_version_id
            for _, observation in content_observations
            if observation.content_version_id is not None
        }
        if not version_ids:
            return {}
        versions = list(
            self._session.scalars(
                select(ContentVersion).where(
                    ContentVersion.owner_id == owner_id,
                    ContentVersion.id.in_(version_ids),
                )
            ).all()
        )
        relations = list(
            self._session.scalars(
                select(ContentVersionRelation).where(
                    ContentVersionRelation.owner_id == owner_id,
                    ContentVersionRelation.content_version_id.in_(version_ids),
                )
            ).all()
        )
        content_by_id = {content.id: content for content, _ in content_observations}
        relation_version = {version.id: version for version in versions}
        target_external_ids = {relation.target_external_id for relation in relations}
        target_source_keys = {
            content_by_id[version.content_id].source_key
            for version in versions
            if version.content_id in content_by_id
        }
        target_candidates = (
            list(
                self._session.scalars(
                    select(ContentRecord).where(
                        ContentRecord.owner_id == owner_id,
                        ContentRecord.object_type == "post",
                        ContentRecord.source_key.in_(target_source_keys),
                        ContentRecord.external_id.in_(target_external_ids),
                    )
                ).all()
            )
            if target_external_ids and target_source_keys
            else []
        )
        readable_target_ids = set(
            self._readable_observations(
                owner_id=owner_id,
                content_ids={candidate.id for candidate in target_candidates},
                now=now,
            )
        )
        readable_targets = {
            (
                candidate.source_key,
                candidate.native_scope,
                candidate.external_id,
            ): candidate.id
            for candidate in target_candidates
            if candidate.id in readable_target_ids
        }
        grouped_relations: dict[UUID, list[ContentVersionRelationView]] = {}
        relation_order = {
            ContentRelationType.QUOTE.value: 0,
            ContentRelationType.REPOST.value: 1,
        }
        for relation in sorted(
            relations,
            key=lambda item: (relation_order[item.relation_type], item.id),
        ):
            version = relation_version[relation.content_version_id]
            source_key = content_by_id[version.content_id].source_key
            grouped_relations.setdefault(relation.content_version_id, []).append(
                ContentVersionRelationView(
                    relation_type=ContentRelationType(relation.relation_type),
                    target_native_scope=relation.target_native_scope,
                    target_external_id=relation.target_external_id,
                    target_author_external_id=relation.target_author_external_id,
                    target_content_id=readable_targets.get(
                        (
                            source_key,
                            relation.target_native_scope,
                            relation.target_external_id,
                        )
                    ),
                )
            )
        return {
            version.id: ContentVersionView(
                id=version.id,
                text_scope=ContentTextScope(version.text_scope),
                text_origin=ContentTextOrigin(version.text_origin),
                text_origin_ref=version.text_origin_ref,
                title=version.title,
                body=version.body,
                truncation_reason=(
                    ContentTruncationReason(version.truncation_reason)
                    if version.truncation_reason
                    else None
                ),
                relations=grouped_relations.get(version.id, []),
            )
            for version in versions
        }

    @staticmethod
    def _version_history(
        observations: list[ContentObservation],
        version_views: Mapping[UUID, ContentVersionView],
    ) -> list[ContentVersionHistoryView]:
        grouped: dict[UUID, list[datetime]] = {}
        for observation in observations:
            version_id = observation.content_version_id
            if version_id is not None and version_id in version_views:
                grouped.setdefault(version_id, []).append(observation.observed_at)
        return sorted(
            (
                ContentVersionHistoryView(
                    content_version=version_views[version_id],
                    first_observed_at=min(observed_at),
                    last_observed_at=max(observed_at),
                    observation_count=len(observed_at),
                )
                for version_id, observed_at in grouped.items()
            ),
            key=lambda item: (item.last_observed_at, item.content_version.id),
            reverse=True,
        )

    @staticmethod
    def _visibility_view(
        observation: ContentVisibilityObservation,
    ) -> ContentVisibilityView:
        return ContentVisibilityView(
            id=observation.id,
            observed_at=observation.observed_at,
            received_at=observation.received_at,
            status=ContentVisibilityStatus(observation.status),
            basis=ContentVisibilityBasis(observation.basis),
        )

    @staticmethod
    def _observation_view(
        observation: ContentObservation,
        content_version: ContentVersionView | None,
    ) -> ContentObservationView:
        return ContentObservationView(
            id=observation.id,
            observed_at=observation.observed_at,
            received_at=observation.received_at,
            published_at=observation.published_at,
            published_at_fractional_digits=observation.published_at_fractional_digits,
            canonical_url=observation.canonical_url,
            final_url=observation.final_url,
            author_external_id=observation.author_external_id,
            metrics=ContentMetricView(
                **{name: getattr(observation, name) for name in _METRIC_FIELDS}
            ),
            content_version=content_version,
        )

    @classmethod
    def _summary_view(
        cls,
        *,
        content: ContentRecord,
        observation: ContentObservation,
        content_version: ContentVersionView | None,
        current_visibility: ContentVisibilityView | None,
        discovery_count: int,
    ) -> ContentRecordSummaryView:
        return ContentRecordSummaryView(
            id=content.id,
            source_key=content.source_key,
            object_type=content.object_type,
            native_scope=content.native_scope,
            external_id=content.external_id,
            latest_observation=cls._observation_view(observation, content_version),
            current_visibility=current_visibility,
            discovery_count=discovery_count,
        )

    @classmethod
    def _detail_view(
        cls,
        *,
        content: ContentRecord,
        observation: ContentObservation,
        content_version: ContentVersionView | None,
        current_visibility: ContentVisibilityView | None,
        discoveries: list[ContentDiscovery],
        job_contexts: dict[UUID, ContentJobContext],
        version_history: list[ContentVersionHistoryView],
        visibility_history: list[ContentVisibilityView],
    ) -> ContentRecordDetailView:
        discovery_views = [
            ContentDiscoveryView(
                job_id=item.job_id,
                configuration_ref=job_contexts[item.job_id].configuration_ref,
                configuration_version=job_contexts[item.job_id].configuration_version,
                first_observed_at=item.first_observed_at,
                scan_kind=job_contexts[item.job_id].scan_kind,
            )
            for item in discoveries
            if item.job_id in job_contexts
        ]
        summary = cls._summary_view(
            content=content,
            observation=observation,
            content_version=content_version,
            current_visibility=current_visibility,
            discovery_count=len(discovery_views),
        )
        return ContentRecordDetailView(
            **summary.model_dump(),
            discoveries=discovery_views,
            version_history=version_history,
            visibility_history=visibility_history,
        )


class ContentObservationCleanup:
    def __init__(self, sessions: sessionmaker[Session]) -> None:
        self._sessions = sessions

    def __call__(self, reference: str) -> None:
        try:
            observation_id = UUID(reference)
        except ValueError as error:
            raise ValueError("content observation cleanup reference must be a UUID") from error
        with self._sessions() as session, session.begin():
            observation = session.scalar(
                select(ContentObservation)
                .where(ContentObservation.id == observation_id)
                .with_for_update()
            )
            if observation is None:
                return
            content_id = observation.content_id
            version_id = observation.content_version_id
            session.delete(observation)
            session.flush()
            remaining_count = session.scalar(
                select(func.count(ContentObservation.id)).where(
                    ContentObservation.content_id == content_id
                )
            )
            if not remaining_count:
                content = session.get(ContentRecord, content_id)
                if content is not None:
                    session.delete(content)
                return
            if version_id is None:
                return
            version_reference_count = session.scalar(
                select(func.count(ContentObservation.id)).where(
                    ContentObservation.content_version_id == version_id
                )
            )
            if not version_reference_count:
                version = session.get(ContentVersion, version_id)
                if version is not None:
                    session.delete(version)


@dataclass(frozen=True, slots=True)
class CommentScanPost:
    owner_id: UUID
    content_id: UUID
    source_key: str
    external_id: str
    created_at: datetime
    title: str | None
    body: str | None
    like_count: int | None
    comment_count: int
    repost_count: int | None
    last_observed_at: datetime | None = None

    @property
    def interaction_score(self) -> int:
        return (self.like_count or 0) + 2 * self.comment_count + 3 * (self.repost_count or 0)

    @property
    def searchable_text(self) -> str:
        return "\n".join(item for item in (self.title, self.body) if item)


@dataclass(frozen=True, slots=True)
class CommentScanCandidate:
    post: CommentScanPost
    topic: ActiveTopicScan
    preset: AppliedSourcePreset


def comment_bucket_start(now: datetime) -> datetime:
    if now.tzinfo is None:
        raise ValueError("comment scan time must be timezone-aware")
    now_utc = now.astimezone(UTC)
    return now_utc.replace(hour=(now_utc.hour // 6) * 6, minute=0, second=0, microsecond=0)


def comment_operation_id(content_id: UUID, bucket_start: datetime) -> UUID:
    if bucket_start.tzinfo is None:
        raise ValueError("comment bucket must be timezone-aware")
    bucket_text = bucket_start.astimezone(UTC).isoformat().replace("+00:00", "Z")
    return uuid5(COMMENT_OPERATION_NAMESPACE, f"comments:{content_id}:{bucket_text}")


def _comment_collection_run(
    candidate: CommentScanCandidate,
    *,
    bucket_start: datetime,
) -> CommentCollectionRunInput:
    return CommentCollectionRunInput(
        operation_id=comment_operation_id(candidate.post.content_id, bucket_start),
        configuration_ref=f"topic:{candidate.topic.topic_id}",
        configuration_version=candidate.topic.topic_version,
        source_key=candidate.post.source_key,
        connection_id=candidate.preset.connection_id,
        connection_version=candidate.preset.connection_version,
        post_external_id=candidate.post.external_id,
        entry_point=SourceEntryPoint.SCHEDULED,
        starts_at=bucket_start - _COMMENT_REFRESH_INTERVAL,
        ends_at=bucket_start,
        scheduled_for_at=bucket_start,
        page_size=20 if candidate.post.source_key == "bilibili" else 100,
        max_pages=1 if candidate.post.source_key == "bilibili" else 10,
        max_requests=1 if candidate.post.source_key == "bilibili" else 10,
    )


def _rank_comment_posts_for_topic(
    *,
    topic: ActiveTopicScan,
    posts: tuple[CommentScanPost, ...],
    presets: Mapping[tuple[UUID, str], AppliedSourcePreset],
    recent_jobs: frozenset[RecentCommentJobTarget],
) -> tuple[CommentScanCandidate, ...]:
    matched: list[CommentScanPost] = []
    for post in posts:
        key = (post.owner_id, post.source_key)
        if (
            post.owner_id != topic.owner_id
            or post.source_key not in topic.source_keys
            or post.comment_count <= 0
            or key not in presets
            or RecentCommentJobTarget(
                owner_id=post.owner_id,
                source_key=post.source_key,
                post_external_id=post.external_id,
            )
            in recent_jobs
            or not evaluate_monitor_rules(topic.rules, post.searchable_text).matched
        ):
            continue
        matched.append(post)
    ranked = sorted(matched, key=lambda item: (-item.interaction_score, str(item.content_id)))
    return tuple(
        CommentScanCandidate(
            post=post,
            topic=topic,
            preset=presets[(post.owner_id, post.source_key)],
        )
        for post in ranked[:_COMMENT_TOPIC_LIMIT]
    )


class CommentScanService:
    """Own candidate selection and comments Job acceptance for the scheduler."""

    def __init__(self, session: Session) -> None:
        self._session = session

    def enqueue_due_comments_in_transaction(
        self, *, now: datetime, skip_bilibili: bool = False
    ) -> int:
        if not self._session.in_transaction():
            raise RuntimeError("comment scanning requires the caller's transaction")
        if now.tzinfo is None:
            raise ValueError("comment scan time must be timezone-aware")
        now_utc = now.astimezone(UTC)
        topics = MonitorScheduleService(
            self._session
        ).list_active_topics_for_scanning_in_transaction()
        presets = self._comment_presets(topics)
        if skip_bilibili:
            presets = {key: preset for key, preset in presets.items() if key[1] != "bilibili"}
        posts = self._load_recent_posts(
            owners={topic.owner_id for topic in topics},
            source_keys={source_key for _, source_key in presets},
            since=now_utc - _COMMENT_POST_LIFETIME,
            until=now_utc,
        )
        recent_jobs = load_recent_comment_job_targets_in_transaction(
            self._session,
            since=now_utc - _COMMENT_REFRESH_INTERVAL,
        )
        # Only a newer Bilibili search refreshes the cached comments. A standalone
        # comment scan must not replay the same JSONL as a fresh platform read.
        for post in posts:
            if post.source_key != "bilibili" or post.last_observed_at is None:
                continue
            target = RecentCommentJobTarget(
                owner_id=post.owner_id,
                source_key="bilibili",
                post_external_id=post.external_id,
            )
            if target in load_recent_comment_job_targets_in_transaction(
                self._session, since=post.last_observed_at
            ):
                recent_jobs |= frozenset({target})
        unique_candidates: dict[tuple[UUID, UUID], CommentScanCandidate] = {}
        for topic in topics:
            for candidate in _rank_comment_posts_for_topic(
                topic=topic,
                posts=posts,
                presets=presets,
                recent_jobs=recent_jobs,
            ):
                unique_candidates.setdefault(
                    (candidate.post.owner_id, candidate.post.content_id), candidate
                )

        from content.comments import build_comment_job_acceptance

        accepted = 0
        bucket_start = comment_bucket_start(now_utc)
        logger = structlog.get_logger("comment_scan")
        for candidate in unique_candidates.values():
            try:
                with self._session.begin_nested():
                    JobService(self._session, clock=lambda: now_utc).accept_in_transaction(
                        owner_id=candidate.post.owner_id,
                        command=build_comment_job_acceptance(
                            _comment_collection_run(candidate, bucket_start=bucket_start)
                        ),
                    )
            except Exception as error:
                logger.warning(
                    "comment_scan_candidate_failed",
                    owner_id=str(candidate.post.owner_id),
                    topic_id=str(candidate.topic.topic_id),
                    content_id=str(candidate.post.content_id),
                    source_key=candidate.post.source_key,
                    error_type=type(error).__name__,
                    exc_info=True,
                )
                continue
            accepted += 1
        return accepted

    def _comment_presets(
        self,
        topics: tuple[ActiveTopicScan, ...],
    ) -> dict[tuple[UUID, str], AppliedSourcePreset]:
        sources_by_owner: dict[UUID, set[str]] = {}
        for topic in topics:
            sources_by_owner.setdefault(topic.owner_id, set()).update(topic.source_keys)
        result: dict[tuple[UUID, str], AppliedSourcePreset] = {}
        for owner_id in sorted(sources_by_owner, key=str):
            applied = load_applied_source_presets_in_transaction(
                self._session,
                owner_id=owner_id,
                source_keys=sorted(sources_by_owner[owner_id]),
            )
            for source_key, preset in applied.items():
                if SourceCapability.COMMENTS in preset.capabilities:
                    result[(owner_id, source_key)] = preset
        return result

    def _load_recent_posts(
        self,
        *,
        owners: set[UUID],
        source_keys: set[str],
        since: datetime,
        until: datetime,
    ) -> tuple[CommentScanPost, ...]:
        if not owners or not source_keys:
            return ()
        latest_versions = select(
            ContentVersion.owner_id.label("owner_id"),
            ContentVersion.content_id.label("content_id"),
            ContentVersion.title.label("title"),
            ContentVersion.body.label("body"),
            func.row_number()
            .over(
                partition_by=(ContentVersion.owner_id, ContentVersion.content_id),
                order_by=(ContentVersion.created_at.desc(), ContentVersion.id.desc()),
            )
            .label("position"),
        ).subquery()
        latest_observations = select(
            ContentObservation.owner_id.label("owner_id"),
            ContentObservation.content_id.label("content_id"),
            ContentObservation.like_count.label("like_count"),
            ContentObservation.comment_count.label("comment_count"),
            ContentObservation.repost_count.label("repost_count"),
            ContentObservation.observed_at.label("observed_at"),
            func.row_number()
            .over(
                partition_by=(ContentObservation.owner_id, ContentObservation.content_id),
                order_by=(
                    ContentObservation.observed_at.desc(),
                    ContentObservation.received_at.desc(),
                    ContentObservation.id.desc(),
                ),
            )
            .label("position"),
        ).subquery()
        rows = self._session.execute(
            select(
                ContentRecord.owner_id,
                ContentRecord.id,
                ContentRecord.source_key,
                ContentRecord.external_id,
                ContentRecord.created_at,
                latest_versions.c.title,
                latest_versions.c.body,
                latest_observations.c.like_count,
                latest_observations.c.comment_count,
                latest_observations.c.repost_count,
                latest_observations.c.observed_at,
            )
            .join(
                latest_versions,
                and_(
                    latest_versions.c.owner_id == ContentRecord.owner_id,
                    latest_versions.c.content_id == ContentRecord.id,
                    latest_versions.c.position == 1,
                ),
            )
            .join(
                latest_observations,
                and_(
                    latest_observations.c.owner_id == ContentRecord.owner_id,
                    latest_observations.c.content_id == ContentRecord.id,
                    latest_observations.c.position == 1,
                ),
            )
            .where(
                ContentRecord.owner_id.in_(owners),
                ContentRecord.source_key.in_(source_keys),
                ContentRecord.object_type == "post",
                or_(
                    ContentRecord.created_at >= since,
                    and_(
                        ContentRecord.source_key == "bilibili",
                        latest_observations.c.observed_at >= since,
                    ),
                ),
                ContentRecord.created_at <= until,
                latest_observations.c.comment_count > 0,
            )
            .order_by(ContentRecord.owner_id, ContentRecord.id)
        ).all()
        return tuple(
            CommentScanPost(
                owner_id=owner_id,
                content_id=content_id,
                source_key=source_key,
                external_id=external_id,
                created_at=(
                    created_at.replace(tzinfo=UTC)
                    if created_at.tzinfo is None
                    else created_at.astimezone(UTC)
                ),
                title=title,
                body=body,
                like_count=like_count,
                comment_count=comment_count,
                repost_count=repost_count,
                last_observed_at=(
                    observed_at.replace(tzinfo=UTC)
                    if observed_at.tzinfo is None
                    else observed_at.astimezone(UTC)
                ),
            )
            for (
                owner_id,
                content_id,
                source_key,
                external_id,
                created_at,
                title,
                body,
                like_count,
                comment_count,
                repost_count,
                observed_at,
            ) in rows
        )


def load_recent_post_versions_for_analysis(
    session: Session,
    *,
    owner_id: UUID,
    since: datetime,
) -> tuple[AnalysisPostContentView, ...]:
    """Read recent post versions without exposing content ORM models cross-domain."""
    if not session.in_transaction():
        raise RuntimeError("analysis content reads require the caller's transaction")
    if since.tzinfo is None:
        raise ValueError("analysis recency boundary must be timezone-aware")
    occurred_at = case(
        (ContentRecord.source_key == "bilibili", ContentObservation.observed_at),
        else_=func.coalesce(ContentObservation.published_at, ContentObservation.observed_at),
    )
    recent_versions = (
        select(
            ContentObservation.content_version_id.label("content_version_id"),
            func.max(occurred_at).label("occurred_at"),
        )
        .join(
            ContentRecord,
            and_(
                ContentRecord.owner_id == ContentObservation.owner_id,
                ContentRecord.id == ContentObservation.content_id,
            ),
        )
        .where(
            ContentObservation.owner_id == owner_id,
            ContentObservation.content_version_id.is_not(None),
            occurred_at >= since,
        )
        .group_by(ContentObservation.content_version_id)
        .subquery()
    )
    rows = session.execute(
        select(ContentVersion, ContentRecord)
        .join(recent_versions, recent_versions.c.content_version_id == ContentVersion.id)
        .join(
            ContentRecord,
            and_(
                ContentRecord.owner_id == ContentVersion.owner_id,
                ContentRecord.id == ContentVersion.content_id,
            ),
        )
        .where(
            ContentVersion.owner_id == owner_id,
            ContentRecord.object_type == "post",
        )
        .order_by(recent_versions.c.occurred_at.desc(), ContentVersion.id)
    ).all()
    return tuple(_analysis_post_view(version, content) for version, content in rows)


def load_post_versions_for_analysis(
    session: Session,
    *,
    owner_id: UUID,
    content_version_ids: set[UUID],
) -> tuple[AnalysisPostContentView, ...]:
    """Read exact immutable post versions frozen into an analysis job."""
    if not session.in_transaction():
        raise RuntimeError("analysis content reads require the caller's transaction")
    if not content_version_ids:
        return ()
    rows = session.execute(
        select(ContentVersion, ContentRecord)
        .join(
            ContentRecord,
            and_(
                ContentRecord.owner_id == ContentVersion.owner_id,
                ContentRecord.id == ContentVersion.content_id,
            ),
        )
        .where(
            ContentVersion.owner_id == owner_id,
            ContentVersion.id.in_(content_version_ids),
            ContentRecord.object_type == "post",
        )
        .order_by(ContentVersion.id)
    ).all()
    return tuple(_analysis_post_view(version, content) for version, content in rows)


def load_post_comments_for_analysis(
    session: Session,
    *,
    owner_id: UUID,
    post_content_ids: set[UUID],
    limit_per_post: int = 50,
) -> dict[UUID, tuple[AnalysisCommentContentView, ...]]:
    """Return each post's newest text for at most 50 distinct comments."""
    if not session.in_transaction():
        raise RuntimeError("analysis comment reads require the caller's transaction")
    if not 1 <= limit_per_post <= 50:
        raise ValueError("analysis comment limit must be between 1 and 50")
    if not post_content_ids:
        return {}

    version_rows = (
        select(
            ContentThread.post_content_id.label("post_content_id"),
            ContentVersion.content_id.label("comment_content_id"),
            ContentVersion.title.label("title"),
            ContentVersion.body.label("body"),
            func.row_number()
            .over(
                partition_by=(ContentVersion.owner_id, ContentVersion.content_id),
                order_by=(ContentVersion.created_at.desc(), ContentVersion.id.desc()),
            )
            .label("version_position"),
        )
        .join(
            ContentVersion,
            and_(
                ContentVersion.owner_id == ContentThread.owner_id,
                ContentVersion.content_id == ContentThread.content_id,
            ),
        )
        .where(
            ContentThread.owner_id == owner_id,
            ContentThread.post_content_id.in_(post_content_ids),
        )
        .subquery()
    )
    latest_versions = (
        select(
            version_rows.c.post_content_id,
            version_rows.c.comment_content_id,
            version_rows.c.title,
            version_rows.c.body,
            func.row_number()
            .over(
                partition_by=version_rows.c.post_content_id,
                order_by=version_rows.c.comment_content_id,
            )
            .label("comment_position"),
        )
        .where(version_rows.c.version_position == 1)
        .subquery()
    )
    rows = session.execute(
        select(
            latest_versions.c.post_content_id,
            latest_versions.c.comment_content_id,
            latest_versions.c.title,
            latest_versions.c.body,
        )
        .where(latest_versions.c.comment_position <= limit_per_post)
        .order_by(
            latest_versions.c.post_content_id,
            latest_versions.c.comment_position,
        )
    ).all()
    comments: dict[UUID, list[AnalysisCommentContentView]] = {}
    for post_content_id, comment_content_id, title, body in rows:
        text = "\n".join(part for part in (title, body) if part)
        if not text:
            continue
        comments.setdefault(post_content_id, []).append(
            AnalysisCommentContentView(
                post_content_id=post_content_id,
                comment_content_id=comment_content_id,
                text=text,
            )
        )
    return {post_id: tuple(items) for post_id, items in comments.items()}


def _analysis_post_view(
    version: ContentVersion,
    content: ContentRecord,
) -> AnalysisPostContentView:
    return AnalysisPostContentView(
        content_id=content.id,
        content_version_id=version.id,
        title=version.title,
        body=version.body,
    )
