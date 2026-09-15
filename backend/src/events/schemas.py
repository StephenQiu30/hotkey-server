from datetime import datetime
from enum import IntEnum
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, Field, field_validator

from core.schemas import Input
from sources.schemas import SourceName


class TrendBucketHours(IntEnum):
    hourly = 1
    six_hour = 6
    daily = 24


class EventInput(Input):
    title: str = Field(min_length=1, max_length=200)
    summary: str = Field(default="", max_length=2000)


class EventMemberInput(Input):
    content_id: UUID


class EventMergeInput(Input):
    source_event_id: UUID
    expected_target_revision: int = Field(ge=1)
    expected_source_revision: int = Field(ge=1)


class EventSplitInput(Input):
    title: str = Field(min_length=1, max_length=200)
    summary: str = Field(default="", max_length=2000)
    content_ids: list[UUID] = Field(min_length=1, max_length=100)
    expected_revision: int = Field(ge=1)

    @field_validator("content_ids")
    @classmethod
    def unique_content_ids(cls, value: list[UUID]) -> list[UUID]:
        if len(value) != len(set(value)):
            raise ValueError("content_ids must be unique")
        return value


class EventMemberView(BaseModel):
    content_id: UUID
    source: SourceName
    kind: Literal["post", "comment", "reply"]
    external_id: str
    canonical_url: str | None
    added_at: datetime


class EventView(BaseModel):
    id: UUID
    title: str
    summary: str
    status: Literal["active", "archived"]
    current_revision: int
    created_at: datetime
    updated_at: datetime
    members: list[EventMemberView]


class EventPage(BaseModel):
    items: list[EventView]
    next_cursor: UUID | None


class EventSnapshot(BaseModel):
    title: str
    summary: str
    status: Literal["active", "archived"]
    member_content_ids: list[UUID]


class EventRevisionView(BaseModel):
    revision: int
    change_type: Literal[
        "create", "add_member", "remove_member", "merge_in", "merge_out", "split_in", "split_out"
    ]
    related_event_id: UUID | None
    snapshot: EventSnapshot
    created_at: datetime


class EventTrendBucket(BaseModel):
    starts_at: datetime
    ends_at: datetime
    new_posts: int = Field(ge=0)
    new_discussions: int = Field(ge=0)
    observed_reply_delta: int
    coverage_status: Literal["comparable", "interrupted", "missing"]
    interruption_reasons: list[
        Literal[
            "policy_changed",
            "run_failed",
            "run_partial",
            "run_incomplete",
            "no_live_coverage",
        ]
    ]
    live_run_count: int = Field(ge=0)
    backfill_run_count: int = Field(ge=0)
    excluded_backfill_items: int = Field(ge=0)
    excluded_backfill_observations: int = Field(ge=0)
    policy_versions: list[str]


class EventSourceTrend(BaseModel):
    source: SourceName
    buckets: list[EventTrendBucket]


class EventTrendView(BaseModel):
    event_id: UUID
    metric_version: Literal["event-trend-v1"] = "event-trend-v1"
    timezone: Literal["UTC"] = "UTC"
    since: datetime
    until: datetime
    bucket_hours: Literal[1, 6, 24]
    sources: list[EventSourceTrend]
