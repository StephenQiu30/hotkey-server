from dataclasses import dataclass
from datetime import UTC, datetime
from hashlib import sha256
from uuid import UUID, uuid4

from sqlalchemy import and_, exists, func, or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from contents.models import Content, ContentObservation, ContentVersion
from contents.schemas import InboxItem, InboxPage
from core.errors import AppError
from monitors.models import MonitorMatch
from monitors.services import monitor_titles_for_content
from sources.schemas import SocialObject


@dataclass(frozen=True)
class ContentWrite:
    content_id: UUID
    new_content: bool
    new_version: bool
    root_content_id: UUID | None


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
                .where(exists(select(MonitorMatch.id).where(MonitorMatch.content_id == Content.id)))
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
