from datetime import UTC, datetime
from uuid import uuid4

import pytest
from sqlalchemy import func, select

from contents.models import Content
from core.errors import AppError
from events.models import EventRevision
from events.schemas import EventInput, EventMemberInput, EventMergeInput
from events.services import EventService
from notifications.models import Notification
from notifications.services import NotificationService, record_event_change

pytestmark = pytest.mark.integration


def test_event_change_notification_is_deduplicated_and_read_state_persists(database):
    now = datetime(2026, 9, 16, 0, 0, tzinfo=UTC)
    content_id = uuid4()
    with database.begin() as session:
        session.add(
            Content(
                id=content_id,
                source="bilibili",
                provider_namespace="video",
                external_id="video:notification",
                kind="post",
                canonical_url=None,
                author_ref="author:synthetic",
                root_external_id="video:notification",
                parent_external_id=None,
                relation_status="root",
                visibility="available",
                first_seen_at=now,
                last_seen_at=now,
            )
        )

    events = EventService(database)
    notifications = NotificationService(database)
    event = events.create(EventInput(title="新品发布"))
    assert notifications.notifications(limit=10, cursor=None, unread_only=False).items == []

    changed = events.add_member(event.id, EventMemberInput(content_id=content_id))
    replayed = events.add_member(event.id, EventMemberInput(content_id=content_id))
    assert replayed.current_revision == changed.current_revision

    page = notifications.notifications(limit=10, cursor=None, unread_only=False)
    assert page.unread_count == 1
    assert len(page.items) == 1
    item = page.items[0]
    assert item.event_id == event.id
    assert item.kind == "event_member_added"
    assert item.read_at is None

    with database.begin() as session:
        revision = session.scalar(
            select(EventRevision).where(
                EventRevision.event_id == event.id,
                EventRevision.revision == changed.current_revision,
            )
        )
        assert revision is not None and item.change_id == revision.id
        record_event_change(
            session,
            event_id=event.id,
            change_id=revision.id,
            change_type="add_member",
            created_at=revision.created_at,
        )
    with database() as session:
        assert session.scalar(select(func.count()).select_from(Notification)) == 1

    read = notifications.mark_read(item.id)
    replayed_read = notifications.mark_read(item.id)
    assert read.read_at is not None and replayed_read.read_at == read.read_at
    persisted = notifications.notifications(limit=10, cursor=None, unread_only=False)
    assert persisted.unread_count == 0
    assert persisted.items[0].read_at == read.read_at


def test_notification_cursor_is_opaque_and_rejects_invalid_values(database):
    events = EventService(database)
    service = NotificationService(database)
    target = events.create(EventInput(title="目标事件"))
    source = events.create(EventInput(title="来源事件"))
    events.merge(
        target.id,
        EventMergeInput(
            source_event_id=source.id,
            expected_target_revision=1,
            expected_source_revision=1,
        ),
    )

    first = service.notifications(limit=1, cursor=None, unread_only=False)
    assert len(first.items) == 1 and first.unread_count == 2
    assert first.next_cursor is not None
    assert first.next_cursor != str(first.items[0].id)
    assert first.items[0].id.hex not in first.next_cursor
    second = service.notifications(limit=1, cursor=first.next_cursor, unread_only=False)
    assert len(second.items) == 1 and second.next_cursor is None
    assert second.items[0].id != first.items[0].id

    with pytest.raises(AppError) as error:
        service.notifications(limit=10, cursor="not-a-valid-cursor", unread_only=False)
    assert error.value.code == "invalid_cursor"
    assert error.value.status == 422
