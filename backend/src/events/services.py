from typing import Literal
from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from contents.models import Content
from core.clock import utcnow
from core.errors import AppError
from events.models import Event, EventMember, EventRevision
from events.schemas import (
    EventInput,
    EventMemberInput,
    EventPage,
    EventRevisionView,
    EventView,
)


class EventService:
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

    @staticmethod
    def _event(session: Session, identity: UUID, *, lock: bool = False) -> Event:
        query = select(Event).where(Event.id == identity)
        event = session.scalar(query.with_for_update() if lock else query)
        if event is None:
            raise AppError("event_not_found", 404)
        return event

    @staticmethod
    def _snapshot(session: Session, event: Event) -> dict[str, object]:
        members = list(
            session.scalars(
                select(EventMember.content_id)
                .where(EventMember.event_id == event.id)
                .order_by(EventMember.content_id)
            )
        )
        return {
            "title": event.title,
            "summary": event.summary,
            "status": event.status,
            "member_content_ids": [str(identity) for identity in members],
        }

    @classmethod
    def _revise(
        cls,
        session: Session,
        event: Event,
        change_type: Literal["create", "add_member", "remove_member"],
    ) -> None:
        session.add(
            EventRevision(
                id=uuid4(),
                event_id=event.id,
                revision=event.current_revision,
                change_type=change_type,
                snapshot=cls._snapshot(session, event),
                created_at=event.updated_at,
            )
        )

    @staticmethod
    def _view(session: Session, event: Event) -> EventView:
        rows = session.execute(
            select(EventMember, Content)
            .join(Content, Content.id == EventMember.content_id)
            .where(EventMember.event_id == event.id)
            .order_by(EventMember.added_at, EventMember.content_id)
        ).all()
        return EventView.model_validate(
            {
                "id": event.id,
                "title": event.title,
                "summary": event.summary,
                "status": event.status,
                "current_revision": event.current_revision,
                "created_at": event.created_at,
                "updated_at": event.updated_at,
                "members": [
                    {
                        "content_id": member.content_id,
                        "source": content.source,
                        "kind": content.kind,
                        "external_id": content.external_id,
                        "canonical_url": content.canonical_url,
                        "added_at": member.added_at,
                    }
                    for member, content in rows
                ],
            }
        )

    def create(self, data: EventInput) -> EventView:
        now = utcnow()
        with self.factory.begin() as session:
            event = Event(
                id=uuid4(),
                title=data.title,
                summary=data.summary,
                status="active",
                current_revision=1,
                created_at=now,
                updated_at=now,
            )
            session.add(event)
            session.flush()
            self._revise(session, event, "create")
            audit(session, "event_created", str(event.id))
            return self._view(session, event)

    def events(self, limit: int, cursor: UUID | None) -> EventPage:
        with self.factory() as session:
            query = select(Event).order_by(Event.id.desc())
            if cursor is not None:
                query = query.where(Event.id < cursor)
            rows = list(session.scalars(query.limit(limit + 1)))
            return EventPage(
                items=[self._view(session, event) for event in rows[:limit]],
                next_cursor=rows[limit - 1].id if len(rows) > limit else None,
            )

    def event(self, identity: UUID) -> EventView:
        with self.factory() as session:
            return self._view(session, self._event(session, identity))

    def add_member(self, identity: UUID, data: EventMemberInput) -> EventView:
        with self.factory.begin() as session:
            event = self._event(session, identity, lock=True)
            content = session.scalar(
                select(Content).where(Content.id == data.content_id).with_for_update()
            )
            if content is None:
                raise AppError("content_not_found", 404)
            assigned = session.scalar(
                select(EventMember.event_id).where(EventMember.content_id == data.content_id)
            )
            if assigned == event.id:
                return self._view(session, event)
            if assigned is not None:
                raise AppError("content_already_assigned", 409)
            now = utcnow()
            session.add(EventMember(event_id=event.id, content_id=data.content_id, added_at=now))
            session.flush()
            event.current_revision += 1
            event.updated_at = now
            self._revise(session, event, "add_member")
            audit(session, "event_member_added", f"{event.id}:{data.content_id}")
            return self._view(session, event)

    def remove_member(self, identity: UUID, content_id: UUID) -> EventView:
        with self.factory.begin() as session:
            event = self._event(session, identity, lock=True)
            member = session.scalar(
                select(EventMember).where(
                    EventMember.event_id == event.id,
                    EventMember.content_id == content_id,
                )
            )
            if member is None:
                raise AppError("event_member_not_found", 404)
            session.delete(member)
            session.flush()
            now = utcnow()
            event.current_revision += 1
            event.updated_at = now
            self._revise(session, event, "remove_member")
            audit(session, "event_member_removed", f"{event.id}:{content_id}")
            return self._view(session, event)

    def revisions(self, identity: UUID) -> list[EventRevisionView]:
        with self.factory() as session:
            self._event(session, identity)
            rows = session.scalars(
                select(EventRevision)
                .where(EventRevision.event_id == identity)
                .order_by(EventRevision.revision)
            )
            return [
                EventRevisionView.model_validate(
                    {
                        "revision": row.revision,
                        "change_type": row.change_type,
                        "snapshot": row.snapshot,
                        "created_at": row.created_at,
                    }
                )
                for row in rows
            ]
