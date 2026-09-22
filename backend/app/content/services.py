from __future__ import annotations

import hashlib
import json
import re
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from enum import StrEnum
from urllib.parse import urlsplit
from uuid import UUID, uuid4

from sqlalchemy import func, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from connections.schemas import (
    ConnectionEvidenceOutcome,
    PersistedReadEvidenceInput,
)
from connections.services import SourceCapabilityEvidenceService
from content.models import (
    ContentDiscovery,
    ContentObservation,
    ContentRecord,
    ContentVersion,
    ContentVersionRelation,
)
from content.schemas import (
    ContentDiscoveryView,
    ContentMetricView,
    ContentObservationView,
    ContentRecordDetailView,
    ContentRecordSummaryView,
    ContentRelationType,
    ContentTextOrigin,
    ContentTextScope,
    ContentTruncationReason,
    ContentVersionRelationView,
    ContentVersionView,
    PersistContentPostInput,
)
from core.errors import ApplicationError
from evidence.services import LifecycleService, load_readable_resource_ids
from jobs.services import (
    ContentJobContext,
    load_content_job_context,
    load_content_job_contexts,
)

type Clock = Callable[[], datetime]

_RESOURCE_TYPE = "content_observation"
_ALLOWED_FIELDS = frozenset(
    {
        "object_type",
        "external_id",
        "canonical_url",
        "author_external_id",
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
_METRIC_FIELDS = (
    "like_count",
    "comment_count",
    "repost_count",
    "view_count",
    "play_count",
    "danmaku_count",
)
_MAX_BIGINT = 9_223_372_036_854_775_807
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


def _optional_url(fields: Mapping[str, object]) -> str | None:
    value = fields.get("canonical_url")
    if value is None:
        return None
    if not isinstance(value, str) or not value or value != value.strip() or len(value) > 2048:
        raise ValueError("canonical_url must be a bounded URL")
    if any(ord(character) < 32 or ord(character) == 127 for character in value):
        raise ValueError("canonical_url cannot contain controls")
    parsed = urlsplit(value)
    if (
        parsed.scheme not in {"http", "https"}
        or parsed.hostname is None
        or parsed.username is not None
        or parsed.password is not None
    ):
        raise ValueError("canonical_url must be an http or https URL without credentials")
    return value


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
        now = self._clock()
        if now.utcoffset() is None:
            raise ValueError("clock must return a timezone-aware datetime")
        if command.admission.owner_id != owner_id:
            raise ApplicationError("resource_not_found")
        if command.admission.collected_at > now:
            raise ValueError("collected_at cannot be in the future")
        fields = dict(command.admission.fields)
        unknown_fields = set(fields) - _ALLOWED_FIELDS
        if unknown_fields:
            raise ValueError("admitted payload contains fields outside the S01 contract")
        object_type = fields.get("object_type", "post")
        if object_type != "post":
            raise ValueError("S01 only accepts post objects")
        external_id = _optional_identifier(fields, "external_id")
        if external_id is None:
            raise ValueError("external_id is required")
        values = self._observation_values(fields)
        version_values = _content_version_values(fields)

        self._session.rollback()
        with self._session.begin():
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
                native_scope=command.native_scope,
                external_id=external_id,
                created_at=now,
            )
            content_version = self._find_or_create_content_version(
                owner_id=owner_id,
                content_id=content.id,
                values=version_values,
                created_at=now,
            )
            values["content_version_id"] = content_version.id if content_version else None
            observation = self._find_or_create_observation(
                owner_id=owner_id,
                content_id=content.id,
                job_id=job.job_id,
                source_operation_id=command.source_operation_id,
                observed_at=command.admission.collected_at,
                received_at=now,
                values=values,
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
                cleanup_targets=[],
            )
            SourceCapabilityEvidenceService(
                self._session,
                clock=self._clock,
            ).record_persisted_read_in_transaction(
                owner_id=owner_id,
                command=PersistedReadEvidenceInput(
                    operation_id=command.source_operation_id,
                    connection_id=command.connection_id,
                    capability=command.admission.capability,
                    entry_point=command.entry_point,
                    outcome=ConnectionEvidenceOutcome.SUCCEEDED,
                    stop_reason=None,
                    resource_ref=f"content_observation:{observation.id}",
                    component_name=command.component_name,
                    component_version=command.component_version,
                ),
            )
            version_views = self._content_version_views(
                owner_id=owner_id,
                content_observations=[(content, observation)],
                now=now,
            )
            view = self._detail_view(
                content=content,
                observation=observation,
                content_version=self._selected_version_view(observation, version_views),
                discoveries=self._discoveries(owner_id, content.id, {job.job_id}),
                job_contexts={job.job_id: job},
            )
        return view

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
            version_views = self._content_version_views(
                owner_id=owner_id,
                content_observations=[(content, observation)],
                now=now,
            )
            return self._detail_view(
                content=content,
                observation=observation,
                content_version=self._selected_version_view(observation, version_views),
                discoveries=discoveries,
                job_contexts=contexts,
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
            items = [
                self._summary_view(
                    content=content,
                    observation=observation,
                    content_version=self._selected_version_view(observation, version_views),
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
            "author_external_id": _optional_identifier(fields, "author_external_id"),
            "published_at": published_at,
            "published_at_fractional_digits": published_at_fractional_digits,
            **{name: _optional_metric(fields, name) for name in _METRIC_FIELDS},
        }

    def _find_or_create_content(
        self,
        *,
        owner_id: UUID,
        source_key: str,
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
                object_type="post",
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
                object_type="post",
                native_scope=native_scope,
                external_id=external_id,
                created_at=created_at,
            )
        conditions = [
            ContentRecord.owner_id == owner_id,
            ContentRecord.source_key == source_key,
            ContentRecord.object_type == "post",
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
        discovery_count: int,
    ) -> ContentRecordSummaryView:
        return ContentRecordSummaryView(
            id=content.id,
            source_key=content.source_key,
            object_type=content.object_type,
            native_scope=content.native_scope,
            external_id=content.external_id,
            latest_observation=cls._observation_view(observation, content_version),
            discovery_count=discovery_count,
        )

    @classmethod
    def _detail_view(
        cls,
        *,
        content: ContentRecord,
        observation: ContentObservation,
        content_version: ContentVersionView | None,
        discoveries: list[ContentDiscovery],
        job_contexts: dict[UUID, ContentJobContext],
    ) -> ContentRecordDetailView:
        discovery_views = [
            ContentDiscoveryView(
                job_id=item.job_id,
                configuration_ref=job_contexts[item.job_id].configuration_ref,
                configuration_version=job_contexts[item.job_id].configuration_version,
                first_observed_at=item.first_observed_at,
            )
            for item in discoveries
            if item.job_id in job_contexts
        ]
        summary = cls._summary_view(
            content=content,
            observation=observation,
            content_version=content_version,
            discovery_count=len(discovery_views),
        )
        return ContentRecordDetailView(
            **summary.model_dump(),
            discoveries=discovery_views,
        )
