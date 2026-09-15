from datetime import UTC, datetime
from uuid import uuid4

import pytest
from sqlalchemy import select

from contents.models import Content
from core.errors import AppError
from events.models import EventRevision
from events.schemas import EventInput, EventMemberInput
from events.services import EventService

pytestmark = pytest.mark.integration


def test_event_membership_is_unique_idempotent_and_revisioned(database):
    content_id = uuid4()
    now = datetime(2026, 9, 15, 12, 0, tzinfo=UTC)
    with database.begin() as session:
        session.add(
            Content(
                id=content_id,
                source="bilibili",
                provider_namespace="video",
                external_id="video:event-1",
                kind="post",
                canonical_url="https://www.bilibili.com/video/BVsynthetic",
                author_ref="author:synthetic",
                root_external_id="video:event-1",
                parent_external_id=None,
                relation_status="root",
                visibility="available",
                first_seen_at=now,
                last_seen_at=now,
            )
        )

    service = EventService(database)
    event = service.create(EventInput(title="品牌发布会", summary="人工整理的事件档案"))
    other = service.create(EventInput(title="另一件事"))
    added = service.add_member(event.id, EventMemberInput(content_id=content_id))
    replayed = service.add_member(event.id, EventMemberInput(content_id=content_id))

    assert added.current_revision == 2
    assert replayed.current_revision == 2
    assert [member.content_id for member in added.members] == [content_id]
    with pytest.raises(AppError) as conflict:
        service.add_member(other.id, EventMemberInput(content_id=content_id))
    assert conflict.value.code == "content_already_assigned"

    removed = service.remove_member(event.id, content_id)
    assert removed.current_revision == 3
    assert removed.members == []
    revisions = service.revisions(event.id)
    assert [revision.change_type for revision in revisions] == [
        "create",
        "add_member",
        "remove_member",
    ]
    assert revisions[1].snapshot.member_content_ids == [content_id]
    assert revisions[2].snapshot.member_content_ids == []
    with database() as session:
        assert list(
            session.scalars(select(EventRevision).where(EventRevision.event_id == event.id))
        )
