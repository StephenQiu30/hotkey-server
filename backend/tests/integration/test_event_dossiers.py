from datetime import UTC, datetime
from uuid import uuid4

import pytest
from sqlalchemy import select

from contents.models import Content
from core.errors import AppError
from events.models import EventRevision
from events.schemas import EventInput, EventMemberInput, EventMergeInput, EventSplitInput
from events.services import EventService

pytestmark = pytest.mark.integration


def content(database, suffix: str):
    identity = uuid4()
    now = datetime(2026, 9, 15, 12, 0, tzinfo=UTC)
    with database.begin() as session:
        session.add(
            Content(
                id=identity,
                source="bilibili",
                provider_namespace="video",
                external_id=f"video:{suffix}",
                kind="post",
                canonical_url=None,
                author_ref="author:synthetic",
                root_external_id=f"video:{suffix}",
                parent_external_id=None,
                relation_status="root",
                visibility="available",
                first_seen_at=now,
                last_seen_at=now,
            )
        )
    return identity


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


def test_merge_then_split_moves_members_and_records_both_sides(database):
    first, second, third = (content(database, str(index)) for index in range(3))
    service = EventService(database)
    target = service.create(EventInput(title="发布会"))
    source = service.create(EventInput(title="同名但不同事件"))
    target = service.add_member(target.id, EventMemberInput(content_id=first))
    target = service.add_member(target.id, EventMemberInput(content_id=second))
    source = service.add_member(source.id, EventMemberInput(content_id=third))

    merged = service.merge(
        target.id,
        EventMergeInput(
            source_event_id=source.id,
            expected_target_revision=target.current_revision,
            expected_source_revision=source.current_revision,
        ),
    )
    assert {member.content_id for member in merged.members} == {first, second, third}
    archived = service.event(source.id)
    assert archived.status == "archived" and archived.members == []
    assert service.revisions(target.id)[-1].change_type == "merge_in"
    assert service.revisions(target.id)[-1].related_event_id == source.id
    assert service.revisions(source.id)[-1].change_type == "merge_out"

    split = service.split(
        target.id,
        EventSplitInput(
            title="售后争议",
            summary="从发布会事件拆出",
            content_ids=[second, third],
            expected_revision=merged.current_revision,
        ),
    )
    original = service.event(target.id)
    assert {member.content_id for member in original.members} == {first}
    assert {member.content_id for member in split.members} == {second, third}
    assert service.revisions(target.id)[-1].change_type == "split_out"
    assert service.revisions(split.id)[0].change_type == "split_in"
    assert service.revisions(split.id)[0].related_event_id == target.id

    with pytest.raises(AppError) as stale:
        service.split(
            target.id,
            EventSplitInput(
                title="过期请求",
                content_ids=[first],
                expected_revision=merged.current_revision,
            ),
        )
    assert stale.value.code == "version_conflict"
