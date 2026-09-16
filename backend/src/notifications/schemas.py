from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, Field

from core.schemas import Input
from sources.schemas import SourceName

TrendAlertMetric = Literal["new_posts", "new_discussions", "observed_reply_delta"]


class TrendAlertRuleInput(Input):
    source: SourceName
    metric: TrendAlertMetric
    bucket_hours: Literal[1, 6, 24]
    threshold_count: int = Field(ge=1, le=1_000_000)


class TrendAlertRuleUpdate(Input):
    expected_version: int = Field(ge=1)
    threshold_count: int = Field(ge=1, le=1_000_000)
    enabled: bool


class TrendAlertRuleView(BaseModel):
    id: UUID
    event_id: UUID
    source: SourceName
    metric: TrendAlertMetric
    bucket_hours: Literal[1, 6, 24]
    threshold_count: int
    version: int
    enabled: bool
    created_at: datetime
    updated_at: datetime


class TrendAlertEvaluationView(BaseModel):
    evaluated_rules: int = Field(ge=0)
    created_notifications: int = Field(ge=0)


class NotificationView(BaseModel):
    id: UUID
    event_id: UUID
    change_id: UUID | None
    trend_occurrence_id: UUID | None
    rule_version: int
    kind: Literal[
        "event_member_added",
        "event_member_removed",
        "event_merged_in",
        "event_merged_out",
        "event_split_in",
        "event_split_out",
        "trend_threshold_reached",
    ]
    message: str
    created_at: datetime
    read_at: datetime | None


class NotificationPage(BaseModel):
    items: list[NotificationView]
    next_cursor: str | None
    unread_count: int
