from typing import Annotated, Literal, cast
from uuid import UUID

from fastapi import APIRouter, Query
from pydantic import AwareDatetime

from api.dependencies import Authenticated, Events
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from events.schemas import (
    EventInput,
    EventMemberInput,
    EventMergeInput,
    EventPage,
    EventRevisionView,
    EventSplitInput,
    EventTrendView,
    EventView,
    TrendBucketHours,
)
from notifications.schemas import (
    TrendAlertEvaluationView,
    TrendAlertRuleInput,
    TrendAlertRuleUpdate,
    TrendAlertRuleView,
)

router = APIRouter(prefix="/events", tags=["events"])


@router.get(
    "",
    response_model=EventPage,
    responses=error_responses(*READ_ERROR_CODES),
    operation_id="listEvents",
)
def events(
    service: Events,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    cursor: UUID | None = None,
) -> EventPage:
    return service.events(limit, cursor)


@router.post(
    "",
    response_model=EventView,
    status_code=201,
    responses=error_responses(*WRITE_ERROR_CODES),
    operation_id="createEvent",
)
def create_event(data: EventInput, service: Events, owner: Authenticated) -> EventView:
    return service.create(data)


@router.get(
    "/{identity}",
    response_model=EventView,
    responses=error_responses(*READ_ERROR_CODES, 404),
    operation_id="getEvent",
)
def get_event(identity: UUID, service: Events, owner: Authenticated) -> EventView:
    return service.event(identity)


@router.post(
    "/{identity}/members",
    response_model=EventView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    operation_id="addEventMember",
)
def add_event_member(
    identity: UUID, data: EventMemberInput, service: Events, owner: Authenticated
) -> EventView:
    return service.add_member(identity, data)


@router.delete(
    "/{identity}/members/{content_id}",
    response_model=EventView,
    responses=error_responses(*WRITE_ERROR_CODES, 404),
    operation_id="removeEventMember",
)
def remove_event_member(
    identity: UUID, content_id: UUID, service: Events, owner: Authenticated
) -> EventView:
    return service.remove_member(identity, content_id)


@router.get(
    "/{identity}/revisions",
    response_model=list[EventRevisionView],
    responses=error_responses(*READ_ERROR_CODES, 404),
    operation_id="listEventRevisions",
)
def event_revisions(
    identity: UUID, service: Events, owner: Authenticated
) -> list[EventRevisionView]:
    return service.revisions(identity)


@router.get(
    "/{identity}/trends",
    response_model=EventTrendView,
    responses=error_responses(*READ_ERROR_CODES, 404, 422),
    operation_id="getEventTrends",
)
def event_trends(
    identity: UUID,
    service: Events,
    owner: Authenticated,
    since: Annotated[AwareDatetime, Query()],
    until: Annotated[AwareDatetime, Query()],
    bucket_hours: TrendBucketHours = TrendBucketHours.daily,
) -> EventTrendView:
    return service.trends(identity, since, until, cast(Literal[1, 6, 24], int(bucket_hours)))


@router.get(
    "/{identity}/trend-alert-rules",
    response_model=list[TrendAlertRuleView],
    responses=error_responses(*READ_ERROR_CODES, 404),
    operation_id="listEventTrendAlertRules",
)
def event_trend_alert_rules(
    identity: UUID, service: Events, owner: Authenticated
) -> list[TrendAlertRuleView]:
    return service.trend_alert_rules(identity)


@router.post(
    "/{identity}/trend-alert-rules",
    response_model=TrendAlertRuleView,
    status_code=201,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    operation_id="createEventTrendAlertRule",
)
def create_event_trend_alert_rule(
    identity: UUID,
    data: TrendAlertRuleInput,
    service: Events,
    owner: Authenticated,
) -> TrendAlertRuleView:
    return service.create_trend_alert_rule(identity, data)


@router.patch(
    "/{identity}/trend-alert-rules/{rule_id}",
    response_model=TrendAlertRuleView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    operation_id="updateEventTrendAlertRule",
)
def update_event_trend_alert_rule(
    identity: UUID,
    rule_id: UUID,
    data: TrendAlertRuleUpdate,
    service: Events,
    owner: Authenticated,
) -> TrendAlertRuleView:
    return service.update_trend_alert_rule(identity, rule_id, data)


@router.post(
    "/{identity}/trend-alerts/evaluate",
    response_model=TrendAlertEvaluationView,
    responses=error_responses(*WRITE_ERROR_CODES, 404),
    operation_id="evaluateEventTrendAlerts",
)
def evaluate_event_trend_alerts(
    identity: UUID, service: Events, owner: Authenticated
) -> TrendAlertEvaluationView:
    return service.evaluate_trend_alerts(identity)


@router.post(
    "/{identity}/merge",
    response_model=EventView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    operation_id="mergeEvent",
)
def merge_event(
    identity: UUID, data: EventMergeInput, service: Events, owner: Authenticated
) -> EventView:
    return service.merge(identity, data)


@router.post(
    "/{identity}/split",
    response_model=EventView,
    status_code=201,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    operation_id="splitEvent",
)
def split_event(
    identity: UUID, data: EventSplitInput, service: Events, owner: Authenticated
) -> EventView:
    return service.split(identity, data)
