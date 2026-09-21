from __future__ import annotations

import pytest

from core.errors import ApplicationError
from monitors.services import evaluate_monitor_rules, normalize_monitor_rules


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
