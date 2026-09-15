from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query

from api.dependencies import Authenticated, Events
from events.schemas import (
    EventInput,
    EventMemberInput,
    EventMergeInput,
    EventPage,
    EventRevisionView,
    EventSplitInput,
    EventView,
)

router = APIRouter(prefix="/api/v1/events", tags=["events"])


@router.get("", response_model=EventPage, operation_id="listEvents")
def events(
    service: Events,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    cursor: UUID | None = None,
) -> EventPage:
    return service.events(limit, cursor)


@router.post("", response_model=EventView, status_code=201, operation_id="createEvent")
def create_event(data: EventInput, service: Events, owner: Authenticated) -> EventView:
    return service.create(data)


@router.get("/{identity}", response_model=EventView, operation_id="getEvent")
def get_event(identity: UUID, service: Events, owner: Authenticated) -> EventView:
    return service.event(identity)


@router.post("/{identity}/members", response_model=EventView, operation_id="addEventMember")
def add_event_member(
    identity: UUID, data: EventMemberInput, service: Events, owner: Authenticated
) -> EventView:
    return service.add_member(identity, data)


@router.delete(
    "/{identity}/members/{content_id}",
    response_model=EventView,
    operation_id="removeEventMember",
)
def remove_event_member(
    identity: UUID, content_id: UUID, service: Events, owner: Authenticated
) -> EventView:
    return service.remove_member(identity, content_id)


@router.get(
    "/{identity}/revisions",
    response_model=list[EventRevisionView],
    operation_id="listEventRevisions",
)
def event_revisions(
    identity: UUID, service: Events, owner: Authenticated
) -> list[EventRevisionView]:
    return service.revisions(identity)


@router.post("/{identity}/merge", response_model=EventView, operation_id="mergeEvent")
def merge_event(
    identity: UUID, data: EventMergeInput, service: Events, owner: Authenticated
) -> EventView:
    return service.merge(identity, data)


@router.post(
    "/{identity}/split", response_model=EventView, status_code=201, operation_id="splitEvent"
)
def split_event(
    identity: UUID, data: EventSplitInput, service: Events, owner: Authenticated
) -> EventView:
    return service.split(identity, data)
