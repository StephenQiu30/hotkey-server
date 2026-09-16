from dataclasses import dataclass
from datetime import UTC, datetime
from hashlib import sha256
from typing import Literal
from uuid import UUID, uuid4

from sqlalchemy import and_, func, or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from contents.models import Content, ContentObservation, ContentVersion, ContentWithdrawalRecord
from contents.schemas import (
    ContentWithdrawalManifest,
    ContentWithdrawalManifestEntry,
    InboxItem,
    InboxPage,
)
from core.clock import utcnow
from core.errors import AppError
from monitors.services import matched_content_ids_query, monitor_titles_for_content
from sources.schemas import SocialObject


@dataclass(frozen=True)
class ContentWrite:
    content_id: UUID
    new_content: bool
    new_version: bool
    root_content_id: UUID | None


@dataclass(frozen=True)
class ContentReference:
    id: UUID
    source: str
    kind: str
    external_id: str
    canonical_url: str | None


@dataclass(frozen=True)
class ContentTrendObservation:
    observed_at: datetime
    reply_count: int | None
    raw_page_id: UUID


@dataclass(frozen=True)
class ContentTrendRecord:
    id: UUID
    source: str
    kind: str
    first_seen_at: datetime
    initial_raw_page_id: UUID | None
    observations: tuple[ContentTrendObservation, ...]


@dataclass(frozen=True)
class AnalysisTextVersion:
    content_id: UUID
    content_version_id: UUID
    text: str
    text_sha256: str
    canonical_url: str | None
    visibility: str


@dataclass(frozen=True)
class ContentAnalysisCandidate:
    content_id: UUID
    content_version_id: UUID
    source: str
    kind: str
    root_external_id: str
    published_at: datetime
    text_sha256: str
    raw_page_id: UUID
    context: AnalysisTextVersion
    parent_context: AnalysisTextVersion | None
    root_context: AnalysisTextVersion | None


@dataclass(frozen=True)
class WithdrawnContent:
    id: UUID
    visibility: str
    version_ids: tuple[UUID, ...]
    raw_page_ids: tuple[UUID, ...]


def _record_withdrawal(
    session: Session,
    *,
    source: str,
    provider_namespace: str,
    external_id: str,
    visibility: Literal["unavailable", "deleted"],
    effective_at: datetime,
) -> ContentWithdrawalRecord:
    record = session.scalar(
        select(ContentWithdrawalRecord)
        .where(
            ContentWithdrawalRecord.source == source,
            ContentWithdrawalRecord.provider_namespace == provider_namespace,
            ContentWithdrawalRecord.external_id == external_id,
        )
        .with_for_update()
    )
    if record is None:
        record = ContentWithdrawalRecord(
            id=uuid4(),
            source=source,
            provider_namespace=provider_namespace,
            external_id=external_id,
            visibility=visibility,
            requested_at=effective_at,
            updated_at=effective_at,
        )
        session.add(record)
    else:
        if record.visibility != "deleted":
            record.visibility = visibility
        record.updated_at = max(record.updated_at, effective_at)
    return record


def export_withdrawal_manifest(session: Session) -> ContentWithdrawalManifest:
    records = list(
        session.scalars(
            select(ContentWithdrawalRecord).order_by(
                ContentWithdrawalRecord.requested_at,
                ContentWithdrawalRecord.id,
            )
        )
    )
    return ContentWithdrawalManifest(
        schema_version="content-withdrawal-manifest-v1",
        generated_at=utcnow(),
        entries=[
            ContentWithdrawalManifestEntry(
                source=record.source,
                provider_namespace=record.provider_namespace,
                external_id=record.external_id,
                visibility=record.visibility,
                effective_at=record.updated_at,
            )
            for record in records
        ],
    )


def apply_withdrawal_manifest_entry(
    session: Session,
    entry: ContentWithdrawalManifestEntry,
) -> UUID | None:
    lock_content_identities(
        session,
        entry.source,
        {(entry.provider_namespace, entry.external_id)},
    )
    _record_withdrawal(
        session,
        source=entry.source,
        provider_namespace=entry.provider_namespace,
        external_id=entry.external_id,
        visibility=entry.visibility,
        effective_at=entry.effective_at,
    )
    content = session.scalar(
        select(Content)
        .where(
            Content.source == entry.source,
            Content.provider_namespace == entry.provider_namespace,
            Content.external_id == entry.external_id,
        )
        .with_for_update()
    )
    return content.id if content is not None else None


def page_contains_withdrawn_content(
    session: Session,
    source: str,
    items: list[SocialObject],
) -> bool:
    identities = {(item.provider_namespace, item.external_id) for item in items}
    if not identities:
        return False
    return (
        session.scalar(
            select(func.count())
            .select_from(ContentWithdrawalRecord)
            .where(
                ContentWithdrawalRecord.source == source,
                or_(
                    *[
                        and_(
                            ContentWithdrawalRecord.provider_namespace == namespace,
                            ContentWithdrawalRecord.external_id == external_id,
                        )
                        for namespace, external_id in identities
                    ]
                ),
            )
        )
        or 0
    ) > 0


def lock_content_identities(
    session: Session,
    source: str,
    identities: set[tuple[str, str]],
) -> None:
    """Serialize collection and withdrawal for stable provider identities."""
    for namespace, external_id in sorted(identities):
        payload = f"{source}\0{namespace}\0{external_id}".encode()
        lock_key = int.from_bytes(sha256(payload).digest()[:8], byteorder="big", signed=True)
        session.execute(select(func.pg_advisory_xact_lock(lock_key)))


def redact_deleted_content_for_raw_page(session: Session, raw_page_id: UUID) -> int:
    content_ids = set(
        session.scalars(
            select(Content.id)
            .join(ContentVersion, ContentVersion.content_id == Content.id)
            .where(
                ContentVersion.raw_page_id == raw_page_id,
                Content.visibility == "deleted",
            )
        )
    )
    if not content_ids:
        return 0
    contents = list(
        session.scalars(select(Content).where(Content.id.in_(content_ids)).with_for_update())
    )
    versions = list(
        session.scalars(
            select(ContentVersion)
            .where(ContentVersion.content_id.in_(content_ids))
            .with_for_update()
        )
    )
    for content in contents:
        content.author_ref = ""
        content.canonical_url = None
        content.parent_external_id = None
    for version in versions:
        marker = f"[deleted:{version.id}]"
        version.text = marker
        version.text_sha256 = sha256(marker.encode()).hexdigest()
    return len(content_ids)


def _latest_versions(
    session: Session, identities: set[UUID], cutoff: datetime
) -> dict[UUID, ContentVersion]:
    if not identities:
        return {}
    latest = (
        select(
            ContentVersion.content_id.label("content_id"),
            func.max(ContentVersion.version).label("version"),
        )
        .where(
            ContentVersion.content_id.in_(identities),
            ContentVersion.observed_at <= cutoff,
        )
        .group_by(ContentVersion.content_id)
        .subquery()
    )
    rows = session.scalars(
        select(ContentVersion).join(
            latest,
            and_(
                latest.c.content_id == ContentVersion.content_id,
                latest.c.version == ContentVersion.version,
            ),
        )
    )
    return {row.content_id: row for row in rows}


def content_version_contexts(
    session: Session, identities: set[UUID]
) -> dict[UUID, AnalysisTextVersion]:
    if not identities:
        return {}
    rows = session.execute(
        select(ContentVersion, Content)
        .join(Content, Content.id == ContentVersion.content_id)
        .where(ContentVersion.id.in_(identities))
    )
    return {
        version.id: AnalysisTextVersion(
            content_id=content.id,
            content_version_id=version.id,
            text=version.text,
            text_sha256=version.text_sha256,
            canonical_url=content.canonical_url,
            visibility=content.visibility,
        )
        for version, content in rows
    }


def current_content_version_ids(session: Session, identities: set[UUID]) -> dict[UUID, UUID]:
    if not identities:
        return {}
    latest = _latest_versions(session, identities, datetime.max.replace(tzinfo=UTC))
    return {content_id: version.id for content_id, version in latest.items()}


def content_analysis_candidates(
    session: Session,
    event_member_ids: tuple[UUID, ...],
    since: datetime,
    until: datetime,
    cutoff: datetime,
) -> list[ContentAnalysisCandidate]:
    if not event_member_ids:
        return []
    members = list(session.scalars(select(Content).where(Content.id.in_(event_member_ids))))
    root_keys = {
        (member.source, member.root_external_id) for member in members if member.kind == "post"
    }
    direct_ids = {member.id for member in members if member.kind in {"comment", "reply"}}
    root_conditions = [
        and_(Content.source == source, Content.root_external_id == root_external_id)
        for source, root_external_id in root_keys
    ]
    scope_conditions = []
    if root_conditions:
        scope_conditions.append(and_(Content.relation_status == "resolved", or_(*root_conditions)))
    if direct_ids:
        scope_conditions.append(Content.id.in_(direct_ids))
    if not scope_conditions:
        return []
    candidate_rows = list(
        session.scalars(
            select(Content).where(
                Content.kind.in_(("comment", "reply")),
                Content.visibility == "available",
                or_(*scope_conditions),
            )
        )
    )
    latest = _latest_versions(session, {row.id for row in candidate_rows}, cutoff)
    candidate_rows = [
        row
        for row in candidate_rows
        if row.id in latest and since <= latest[row.id].published_at < until
    ]
    if not candidate_rows:
        return []

    parent_keys = {
        (row.source, row.provider_namespace, row.parent_external_id)
        for row in candidate_rows
        if row.parent_external_id is not None
    }
    parent_rows = (
        list(
            session.scalars(
                select(Content).where(
                    or_(
                        *[
                            and_(
                                Content.source == source,
                                Content.provider_namespace == namespace,
                                Content.external_id == external_id,
                            )
                            for source, namespace, external_id in parent_keys
                        ]
                    )
                )
            )
        )
        if parent_keys
        else []
    )
    root_rows = list(
        session.scalars(
            select(Content).where(
                Content.kind == "post",
                or_(
                    *[
                        and_(
                            Content.source == source,
                            Content.external_id == root_external_id,
                        )
                        for source, root_external_id in {
                            (row.source, row.root_external_id) for row in candidate_rows
                        }
                    ]
                ),
            )
        )
    )
    context_versions = _latest_versions(
        session, {row.id for row in [*parent_rows, *root_rows]}, cutoff
    )
    parent_contexts = {
        (row.source, row.provider_namespace, row.external_id): AnalysisTextVersion(
            content_id=row.id,
            content_version_id=context_versions[row.id].id,
            text=context_versions[row.id].text,
            text_sha256=context_versions[row.id].text_sha256,
            canonical_url=row.canonical_url,
            visibility=row.visibility,
        )
        for row in parent_rows
        if row.id in context_versions
    }
    roots_by_key: dict[tuple[str, str], list[Content]] = {}
    for row in root_rows:
        roots_by_key.setdefault((row.source, row.external_id), []).append(row)
    root_contexts = {}
    for key, rows in roots_by_key.items():
        if len(rows) != 1 or rows[0].id not in context_versions:
            continue
        row = rows[0]
        root_contexts[key] = AnalysisTextVersion(
            content_id=row.id,
            content_version_id=context_versions[row.id].id,
            text=context_versions[row.id].text,
            text_sha256=context_versions[row.id].text_sha256,
            canonical_url=row.canonical_url,
            visibility=row.visibility,
        )

    candidates: list[ContentAnalysisCandidate] = []
    for row in candidate_rows:
        version = latest[row.id]
        own = AnalysisTextVersion(
            content_id=row.id,
            content_version_id=version.id,
            text=version.text,
            text_sha256=version.text_sha256,
            canonical_url=row.canonical_url,
            visibility=row.visibility,
        )
        candidates.append(
            ContentAnalysisCandidate(
                content_id=row.id,
                content_version_id=version.id,
                source=row.source,
                kind=row.kind,
                root_external_id=row.root_external_id,
                published_at=version.published_at,
                text_sha256=version.text_sha256,
                raw_page_id=version.raw_page_id,
                context=own,
                parent_context=(
                    parent_contexts.get(
                        (row.source, row.provider_namespace, row.parent_external_id)
                    )
                    if row.parent_external_id is not None
                    else None
                ),
                root_context=root_contexts.get((row.source, row.root_external_id)),
            )
        )
    return sorted(
        candidates,
        key=lambda item: (
            item.source,
            item.root_external_id,
            item.published_at,
            item.content_version_id,
        ),
    )


def content_trend_records(
    session: Session, identities: list[UUID], until: datetime
) -> list[ContentTrendRecord]:
    if not identities:
        return []
    contents = list(session.scalars(select(Content).where(Content.id.in_(identities))))
    initial_pages: dict[UUID, UUID] = {
        content_id: raw_page_id
        for content_id, raw_page_id in session.execute(
            select(ContentVersion.content_id, ContentVersion.raw_page_id).where(
                ContentVersion.content_id.in_(identities), ContentVersion.version == 1
            )
        )
    }
    observations: dict[UUID, list[ContentTrendObservation]] = {
        identity: [] for identity in identities
    }
    rows = session.scalars(
        select(ContentObservation)
        .where(
            ContentObservation.content_id.in_(identities),
            ContentObservation.observed_at < until,
        )
        .order_by(ContentObservation.content_id, ContentObservation.observed_at)
    )
    for row in rows:
        observations[row.content_id].append(
            ContentTrendObservation(
                observed_at=row.observed_at,
                reply_count=row.reply_count,
                raw_page_id=row.raw_page_id,
            )
        )
    return [
        ContentTrendRecord(
            id=content.id,
            source=content.source,
            kind=content.kind,
            first_seen_at=content.first_seen_at,
            initial_raw_page_id=initial_pages.get(content.id),
            observations=tuple(observations[content.id]),
        )
        for content in contents
    ]


def content_references(
    session: Session, identities: list[UUID], *, lock: bool = False
) -> dict[UUID, ContentReference]:
    if not identities:
        return {}
    query = select(Content).where(Content.id.in_(identities))
    rows = session.scalars(query.with_for_update() if lock else query)
    return {
        content.id: ContentReference(
            id=content.id,
            source=content.source,
            kind=content.kind,
            external_id=content.external_id,
            canonical_url=content.canonical_url,
        )
        for content in rows
    }


def withdraw_content(
    session: Session,
    identity: UUID,
    reason: Literal["deleted", "purpose_revoked"],
) -> WithdrawnContent:
    content = session.get(Content, identity)
    if content is None:
        raise AppError("content_not_found", 404)
    lock_content_identities(
        session,
        content.source,
        {(content.provider_namespace, content.external_id)},
    )
    content = session.scalar(select(Content).where(Content.id == identity).with_for_update())
    if content is None:
        raise AppError("content_not_found", 404)
    requested_visibility: Literal["unavailable", "deleted"] = (
        "deleted" if reason == "deleted" else "unavailable"
    )
    if content.visibility != "deleted":
        content.visibility = requested_visibility
    final_visibility: Literal["unavailable", "deleted"] = (
        "deleted" if content.visibility == "deleted" else "unavailable"
    )
    now = utcnow()
    _record_withdrawal(
        session,
        source=content.source,
        provider_namespace=content.provider_namespace,
        external_id=content.external_id,
        visibility=final_visibility,
        effective_at=now,
    )
    version_ids = tuple(
        session.scalars(
            select(ContentVersion.id)
            .where(ContentVersion.content_id == identity)
            .order_by(ContentVersion.version)
        )
    )
    raw_page_ids = tuple(
        sorted(
            {
                *session.scalars(
                    select(ContentVersion.raw_page_id).where(ContentVersion.content_id == identity)
                ),
                *session.scalars(
                    select(ContentObservation.raw_page_id).where(
                        ContentObservation.content_id == identity
                    )
                ),
            }
        )
    )
    return WithdrawnContent(
        id=content.id,
        visibility=content.visibility,
        version_ids=version_ids,
        raw_page_ids=raw_page_ids,
    )


def upsert_content(
    session: Session,
    item: SocialObject,
    source: str,
    raw_page_id: UUID,
    observed_at: datetime,
) -> ContentWrite:
    identity = {
        "source": source,
        "provider_namespace": item.provider_namespace,
        "external_id": item.external_id,
    }
    content_id = session.scalar(
        insert(Content)
        .values(
            id=uuid4(),
            **identity,
            kind=item.kind,
            canonical_url=item.canonical_url,
            author_ref=item.author_id,
            root_external_id=item.root_id,
            parent_external_id=item.parent_id,
            relation_status="root" if item.kind == "post" else "unresolved",
            visibility="available",
            first_seen_at=observed_at,
            last_seen_at=observed_at,
        )
        .on_conflict_do_nothing(
            index_elements=[Content.source, Content.provider_namespace, Content.external_id]
        )
        .returning(Content.id)
    )
    new_content = content_id is not None
    content = session.scalar(
        select(Content)
        .where(*[getattr(Content, key) == value for key, value in identity.items()])
        .with_for_update()
    )
    assert content is not None
    content.last_seen_at = max(content.last_seen_at, observed_at)
    if item.canonical_url is not None:
        content.canonical_url = item.canonical_url
    root_content_id = None
    if item.kind != "post":
        root_candidates = list(
            session.scalars(
                select(Content.id)
                .where(
                    Content.source == source,
                    Content.kind == "post",
                    Content.external_id == item.root_id,
                )
                .limit(2)
            )
        )
        root_content_id = root_candidates[0] if len(root_candidates) == 1 else None
        parent_exists = item.parent_id is None
        if item.parent_id is not None:
            parent_candidates = list(
                session.scalars(
                    select(Content.id)
                    .where(
                        Content.source == source,
                        Content.kind.in_(("comment", "reply")),
                        Content.external_id == item.parent_id,
                    )
                    .limit(2)
                )
            )
            parent_exists = len(parent_candidates) == 1
        content.relation_status = (
            "resolved" if root_content_id is not None and parent_exists else "unresolved"
        )

    text_hash = sha256(item.text.encode()).hexdigest()
    version = session.scalar(
        select(ContentVersion).where(
            ContentVersion.content_id == content.id, ContentVersion.text_sha256 == text_hash
        )
    )
    new_version = version is None
    if new_version:
        next_version = (
            session.scalar(
                select(func.max(ContentVersion.version)).where(
                    ContentVersion.content_id == content.id
                )
            )
            or 0
        ) + 1
        session.add(
            ContentVersion(
                id=uuid4(),
                content_id=content.id,
                version=next_version,
                text=item.text,
                text_sha256=text_hash,
                published_at=item.created_at,
                observed_at=observed_at,
                raw_page_id=raw_page_id,
            )
        )
    session.execute(
        insert(ContentObservation)
        .values(
            id=uuid4(),
            content_id=content.id,
            observed_at=observed_at,
            reply_count=item.reply_count,
            raw_page_id=raw_page_id,
        )
        .on_conflict_do_nothing(
            index_elements=[ContentObservation.content_id, ContentObservation.raw_page_id]
        )
    )
    matched_root_content_id = root_content_id if content.relation_status == "resolved" else None
    return ContentWrite(content.id, new_content, new_version, matched_root_content_id)


class ContentService:
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

    @staticmethod
    def _decode_cursor(cursor: str) -> tuple[datetime, UUID]:
        try:
            timestamp, identity = cursor.rsplit("|", 1)
            first_seen_at = datetime.fromisoformat(timestamp)
            if first_seen_at.tzinfo is None:
                raise ValueError("cursor timestamp must include a timezone")
            return first_seen_at.astimezone(UTC), UUID(identity)
        except (TypeError, ValueError) as error:
            raise AppError("invalid_cursor", 422) from error

    @staticmethod
    def _encode_cursor(content: Content) -> str:
        return f"{content.first_seen_at.astimezone(UTC).isoformat()}|{content.id}"

    def inbox(self, limit: int, cursor: str | None) -> InboxPage:
        with self.factory() as session:
            query = (
                select(Content)
                .where(
                    Content.id.in_(matched_content_ids_query()),
                    Content.visibility == "available",
                )
                .order_by(Content.first_seen_at.desc(), Content.id.desc())
            )
            if cursor is not None:
                first_seen_at, identity = self._decode_cursor(cursor)
                query = query.where(
                    or_(
                        Content.first_seen_at < first_seen_at,
                        and_(Content.first_seen_at == first_seen_at, Content.id < identity),
                    )
                )
            rows = list(session.scalars(query.limit(limit + 1)))
            items = []
            for content in rows[:limit]:
                version = session.scalar(
                    select(ContentVersion)
                    .where(ContentVersion.content_id == content.id)
                    .order_by(ContentVersion.version.desc())
                    .limit(1)
                )
                observation = session.scalar(
                    select(ContentObservation)
                    .where(ContentObservation.content_id == content.id)
                    .order_by(ContentObservation.observed_at.desc())
                    .limit(1)
                )
                assert version is not None
                items.append(
                    InboxItem(
                        id=content.id,
                        source=content.source,
                        provider_namespace=content.provider_namespace,
                        external_id=content.external_id,
                        kind=content.kind,
                        root_external_id=content.root_external_id,
                        parent_external_id=content.parent_external_id,
                        relation_status=content.relation_status,
                        text=version.text,
                        version=version.version,
                        canonical_url=content.canonical_url,
                        published_at=version.published_at,
                        first_seen_at=content.first_seen_at,
                        last_seen_at=content.last_seen_at,
                        reply_count=observation.reply_count if observation else None,
                        monitor_titles=monitor_titles_for_content(session, content.id),
                    )
                )
            return InboxPage(
                items=items,
                next_cursor=self._encode_cursor(rows[limit - 1]) if len(rows) > limit else None,
            )
