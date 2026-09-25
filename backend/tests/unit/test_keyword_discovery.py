from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from pydantic import ValidationError

from connections.schemas import SourceEntryPoint
from content.discovery import plan_keyword_discovery, plan_scheduled_keyword_discovery
from content.schemas import KeywordDiscoveryRunInput


def _run(**changes: object) -> KeywordDiscoveryRunInput:
    values: dict[str, object] = {
        "run_id": uuid4(),
        "configuration_ref": "topic:fixture",
        "configuration_version": 3,
        "source_key": "x",
        "connection_id": uuid4(),
        "connection_version": 1,
        "primary_query": "product fault",
        "upstream_aliases": ("product issue",),
        "starts_at": datetime(2026, 9, 22, tzinfo=UTC),
        "ends_at": datetime(2026, 9, 23, tzinfo=UTC),
        "page_size": 20,
        "latest_max_pages": 3,
        "latest_max_requests": 6,
        "top_max_pages": 2,
        "top_max_requests": 4,
        "max_seconds": 30,
        "entry_point": SourceEntryPoint.MANUAL,
    }
    values.update(changes)
    return KeywordDiscoveryRunInput.model_validate(values)


def test_plan_freezes_only_explicit_upstream_queries_with_independent_channels() -> None:
    run = _run()
    planned = plan_keyword_discovery(run)

    assert len(planned) == 4
    assert len({job.operation_id for job in planned}) == 4
    assert {job.scope["run_id"] for job in planned} == {str(run.run_id)}
    assert {job.scope["query"] for job in planned} == {"product fault", "product issue"}
    assert {job.scope["rule_version"] for job in planned} == {3}
    assert {job.scope["sort_key"] for job in planned} == {"latest", "top"}
    assert {job.scope["max_pages"] for job in planned if job.scope["sort_key"] == "latest"} == {3}
    assert {job.scope["max_pages"] for job in planned if job.scope["sort_key"] == "top"} == {2}
    assert {job.scope["max_requests"] for job in planned if job.scope["sort_key"] == "latest"} == {
        6
    }
    assert {job.scope["max_requests"] for job in planned if job.scope["sort_key"] == "top"} == {4}
    assert {job.scope["max_seconds"] for job in planned} == {30}
    assert {job.scope["entry_point"] for job in planned} == {"manual"}
    assert all(job.kind == "keyword.search" for job in planned)
    assert all(job.observation.configuration_version == 3 for job in planned)
    assert {job.scope["query"] for job in plan_keyword_discovery(_run(upstream_aliases=()))} == {
        "product fault"
    }
    assert plan_keyword_discovery(run) == planned


def test_same_query_under_different_topics_has_distinct_coverage_target() -> None:
    first = plan_keyword_discovery(_run(configuration_ref="topic:first"))
    second = plan_keyword_discovery(_run(configuration_ref="topic:second"))
    assert first[0].scope["target_hash"] != second[0].scope["target_hash"]


def test_scheduled_plan_uses_run_id_as_the_single_operation_and_carries_entry_point() -> None:
    scheduled_for_at = datetime(2026, 9, 23, tzinfo=UTC)
    run = _run(
        entry_point=SourceEntryPoint.SCHEDULED,
        scheduled_for_at=scheduled_for_at,
    )

    command = plan_scheduled_keyword_discovery(run)

    assert command.operation_id == run.run_id
    assert command.scheduled_for_at == scheduled_for_at
    assert command.scope["entry_point"] == "scheduled"
    assert command.scope["sort_key"] == "latest"


def test_scheduled_plan_rejects_manual_entry_point() -> None:
    with pytest.raises(ValueError, match="scheduled entry point"):
        plan_scheduled_keyword_discovery(_run())


@pytest.mark.parametrize(
    "changes",
    [
        {"starts_at": datetime(2026, 9, 23, tzinfo=UTC)},
        {"ends_at": datetime(2026, 9, 21, tzinfo=UTC)},
        {"ends_at": datetime(2026, 9, 23)},
        {"latest_max_pages": 0},
        {"top_max_pages": 0},
        {"latest_max_requests": 0},
        {"top_max_requests": 101},
        {"max_seconds": 0},
        {"max_seconds": 91},
        {"page_size": 101},
        {"primary_query": " bad"},
        {"upstream_aliases": ("product fault",)},
        {"upstream_aliases": ("product issue", "product issue")},
        {"upstream_aliases": ("\uff50\uff52\uff4f\uff44\uff55\uff43\uff54 fault",)},
        {"upstream_aliases": ("alias",) * 10},
        {"ends_at": datetime(2026, 9, 22, tzinfo=UTC) + timedelta(days=31)},
    ],
)
def test_plan_rejects_ambiguous_or_unbounded_queries(changes: dict[str, object]) -> None:
    with pytest.raises(ValidationError):
        _run(**changes)
