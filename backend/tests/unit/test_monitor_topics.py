from __future__ import annotations

from datetime import time
from typing import cast

import pytest
from pydantic import ValidationError
from sqlalchemy import ForeignKeyConstraint, PrimaryKeyConstraint, Table, UniqueConstraint
from sqlalchemy.orm import Session

from core.errors import ApplicationError
from monitors.models import MonitorSchedule, MonitorTopic
from monitors.schemas import MonitorTopicCreateInput, MonitorTopicPreviewInput
from monitors.services import (
    MonitorTopicService,
    evaluate_monitor_rules,
    normalize_monitor_rules,
)


def test_topic_settings_default_to_half_hour_shanghai_daily_report() -> None:
    command = MonitorTopicCreateInput(
        name="品牌动态",
        match_any=["品牌"],
        match_all=[],
        exclude=[],
    )

    assert command.source_keys == []
    assert command.collection_interval_seconds == 1800
    assert command.report_time == time(hour=9)
    assert command.weekly_report_enabled is False
    assert command.notification_target_names == []


@pytest.mark.parametrize("interval_seconds", [599, 86401])
def test_topic_settings_reject_out_of_range_collection_interval(
    interval_seconds: int,
) -> None:
    with pytest.raises(ValidationError):
        MonitorTopicCreateInput(
            name="品牌动态",
            match_any=["品牌"],
            match_all=[],
            exclude=[],
            collection_interval_seconds=interval_seconds,
        )


def test_topic_settings_reject_invalid_source_keys_and_target_limits() -> None:
    with pytest.raises(ValidationError):
        MonitorTopicCreateInput(
            name="品牌动态",
            match_any=["品牌"],
            match_all=[],
            exclude=[],
            source_keys=["Not Valid"],
        )

    with pytest.raises(ValidationError):
        MonitorTopicCreateInput(
            name="品牌动态",
            match_any=["品牌"],
            match_all=[],
            exclude=[],
            notification_target_names=[f"目标 {index}" for index in range(21)],
        )

    with pytest.raises(ValidationError):
        MonitorTopicCreateInput(
            name="品牌动态",
            match_any=["品牌"],
            match_all=[],
            exclude=[],
            report_time="09:00+08:00",  # type: ignore[arg-type]
        )


def test_monitor_schedule_uses_owner_scoped_topic_identity_and_schedule_key() -> None:
    schedule_constraints = cast(Table, MonitorSchedule.__table__).constraints
    topic_constraints = cast(Table, MonitorTopic.__table__).constraints

    assert any(
        isinstance(constraint, UniqueConstraint)
        and constraint.name == "monitor_topics_owner_id_key"
        for constraint in topic_constraints
    )
    assert any(
        isinstance(constraint, ForeignKeyConstraint)
        and constraint.name == "monitor_schedules_owner_topic_fkey"
        and tuple(constraint.column_keys) == ("owner_id", "topic_id")
        for constraint in schedule_constraints
    )
    assert any(
        isinstance(constraint, PrimaryKeyConstraint)
        and constraint.name == "monitor_schedules_owner_topic_source_capability_key"
        and tuple(column.name for column in constraint.columns)
        == ("owner_id", "topic_id", "source_key", "capability")
        for constraint in schedule_constraints
    )


@pytest.mark.parametrize(
    ("match_any", "match_all", "exclude", "title", "matched", "excluded_by"),
    [
        (["Brand"], [], [], "brand 召回", True, ()),
        (["\uff22\uff32\uff21\uff2e\uff24"], [], [], "brand 召回", True, ()),
        (["  品牌\t召回  "], [], [], "品牌 召回通知", True, ()),
        (["华为", "HUAWEI"], [], [], "Huawei 发布会", True, ()),
        (["品牌", "厂商"], [], [], "厂商公告", True, ()),
        ([], ["品牌", "召回"], [], "品牌新闻", False, ()),
        ([], ["品牌", "召回"], [], "品牌召回公告", True, ()),
        ([], ["品牌"], [], "品牌公告", True, ()),
        (["品牌"], [], [], "品牌公告", True, ()),
        (["品牌", "召回"], [], ["招聘"], "品牌召回招聘", False, ("招聘",)),
    ],
)
def test_frozen_rule_samples_match_with_exclusion_priority(
    match_any: list[str],
    match_all: list[str],
    exclude: list[str],
    title: str,
    matched: bool,
    excluded_by: tuple[str, ...],
) -> None:
    rules = normalize_monitor_rules(
        match_any=match_any,
        match_all=match_all,
        exclude=exclude,
    )

    result = evaluate_monitor_rules(rules, title)

    assert result.matched is matched
    assert result.excluded_by == excluded_by


def test_empty_include_groups_are_rejected_instead_of_matching_everything() -> None:
    with pytest.raises(ApplicationError, match="invalid_monitor_rules"):
        normalize_monitor_rules(match_any=[], match_all=[], exclude=[])


def test_keywords_are_stably_deduplicated_and_cross_group_conflicts_are_rejected() -> None:
    rules = normalize_monitor_rules(
        match_any=["品牌", "\uff22\uff32\uff21\uff2e\uff24", "brand"],
        match_all=[],
        exclude=[],
    )

    assert rules.match_any == ("品牌", "BRAND")

    with pytest.raises(ApplicationError, match="keyword_group_conflict"):
        normalize_monitor_rules(
            match_any=["品牌"],
            match_all=[],
            exclude=[" 品牌 "],
        )


def test_same_keyword_cannot_appear_in_any_and_all_groups() -> None:
    with pytest.raises(ApplicationError, match="keyword_group_conflict"):
        normalize_monitor_rules(
            match_any=["Brand"],
            match_all=["\uff22\uff32\uff21\uff2e\uff24"],
            exclude=[],
        )


def test_rule_match_reports_actual_positive_and_excluded_terms_in_rule_order() -> None:
    rules = normalize_monitor_rules(
        match_any=["Brand", "发布", "公告"],
        match_all=["新品", "官方"],
        exclude=["招聘"],
    )

    result = evaluate_monitor_rules(rules, "发布 BRAND 新品 招聘")

    assert result.matched is False
    assert result.matched_any == ("Brand", "发布")
    assert result.matched_all == ("新品",)
    assert result.excluded_by == ("招聘",)


def test_topic_preview_returns_explanation_for_each_sample_without_database_io() -> None:
    with Session() as session:
        preview = MonitorTopicService(session).preview_topic(
            command=MonitorTopicPreviewInput(
                match_any=["Brand", "发布"],
                match_all=["新品", "官方"],
                exclude=["招聘"],
                sample_titles=["Brand 发布新品 招聘"],
            )
        )

    assert preview.samples[0].matched is False
    assert preview.samples[0].matched_any == ["Brand", "发布"]
    assert preview.samples[0].matched_all == ["新品"]
    assert preview.samples[0].excluded_by == ["招聘"]
