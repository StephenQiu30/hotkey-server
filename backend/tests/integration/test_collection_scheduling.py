from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime
from uuid import uuid4

import pytest
from sqlalchemy import func, select

from collection.models import CollectionBudgetUsage, CollectionRun
from collection.scheduling import CollectionScheduler
from collection.schemas import CollectionRunBatchInput, CollectionRunInput
from collection.services import CollectionService
from core.clock import utcnow
from core.errors import AppError
from jobs.models import Job, Outbox
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


def active_monitor(database, *, daily_requests=4, aliases=None):
    sources = AdmittedSources()
    service = MonitorService(database, sources)
    created = service.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "周期预算",
                "query_spec": {"include_any": ["AI"], "aliases": aliases or []},
                "source_ids": ["bilibili"],
                "schedule": {"interval_minutes": 1440},
                "budget": {
                    "daily_requests": daily_requests,
                    "content_purchase_cost": 0,
                },
            }
        )
    )
    return service.change_state(
        created.id,
        MonitorStateChange(expected_version=created.current_version),
        "active",
    )


def manual_run(monitor, key):
    return CollectionRunInput(
        monitor_id=monitor.id,
        expected_version=monitor.current_version,
        source="bilibili",
        request_value="AI",
        since="2026-09-14T00:00:00Z",
        until="2026-09-15T00:00:00Z",
        idempotency_key=key,
        policy_version="policy-v1",
        retention_days=7,
        ingestion_mode="live",
    )


def test_request_budget_is_atomic_across_concurrent_run_keys(database):
    sources = AdmittedSources()
    monitor = active_monitor(database)

    def create(index):
        try:
            return CollectionService(database, sources, evidence_configured=True).create_run(
                manual_run(monitor, f"budget-concurrent-{index}")
            )
        except AppError as error:
            return error.code

    with ThreadPoolExecutor(max_workers=4) as pool:
        results = list(pool.map(create, range(8)))
    assert sum(not isinstance(result, str) for result in results) == 4
    assert results.count("request_budget_exhausted") == 4
    with database() as session:
        usage = session.scalar(select(CollectionBudgetUsage))
        assert usage is not None and usage.reserved_requests == 4
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 4
        assert session.scalar(select(func.count()).select_from(Job)) == 4
        assert session.scalar(select(func.count()).select_from(Outbox)) == 4


def test_manual_batch_creates_all_queries_atomically_and_replays(database):
    sources = AdmittedSources()
    monitor = active_monitor(database, daily_requests=100, aliases=["人工智能"])
    service = CollectionService(database, sources, evidence_configured=True)
    request = CollectionRunBatchInput(
        monitor_id=monitor.id,
        expected_version=monitor.current_version,
        idempotency_key="manual-batch-one",
        trigger="manual",
        ingestion_mode="live",
    )

    created = service.create_monitor_runs(request)
    replayed = service.create_monitor_runs(request)

    assert created.replayed is False
    assert replayed.replayed is True
    assert [run.id for run in replayed.items] == [run.id for run in created.items]
    assert [run.request_value for run in created.items] == ["AI", "人工智能"]
    assert len({run.window_since for run in created.items}) == 1
    assert len({run.window_until for run in created.items}) == 1
    assert {run.retention_days for run in created.items} == {7}
    with database() as session:
        usage = session.scalar(select(CollectionBudgetUsage))
        assert usage is not None and usage.reserved_requests == 2
        rows = list(session.scalars(select(CollectionRun)))
        assert len(rows) == 2
        assert {run.policy_version for run in rows} == {
            f"monitor-version-{monitor.current_version}"
        }
        assert session.scalar(select(func.count()).select_from(Job)) == 2
        assert session.scalar(select(func.count()).select_from(Outbox)) == 2


def test_manual_batch_budget_failure_rolls_back_entire_batch(database):
    sources = AdmittedSources()
    monitor = active_monitor(database, daily_requests=100, aliases=["人工智能"])
    service = CollectionService(database, sources, evidence_configured=True)
    configuration = MonitorService(database, sources).active_configurations()[0]
    now = utcnow()
    with database.begin() as session:
        session.add(
            CollectionBudgetUsage(
                id=uuid4(),
                monitor_version_id=configuration.monitor_version_id,
                budget_day=now.date(),
                limit_requests=100,
                reserved_requests=99,
                created_at=now,
                updated_at=now,
            )
        )
    with pytest.raises(AppError) as error:
        service.create_monitor_runs(
            CollectionRunBatchInput(
                monitor_id=monitor.id,
                expected_version=monitor.current_version,
                idempotency_key="manual-batch-too-large",
                trigger="manual",
                ingestion_mode="live",
            )
        )
    assert error.value.code == "request_budget_exhausted"
    with database() as session:
        usage = session.scalar(select(CollectionBudgetUsage))
        assert usage is not None and usage.reserved_requests == 99
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 0
        assert session.scalar(select(func.count()).select_from(Job)) == 0
        assert session.scalar(select(func.count()).select_from(Outbox)) == 0


def test_scheduler_creates_only_latest_completed_slot_and_is_idempotent(database):
    sources = AdmittedSources()
    monitor = active_monitor(database, daily_requests=4)
    scheduler = CollectionScheduler(database, sources, evidence_configured=True)
    now = datetime(2026, 9, 15, 12, 34, tzinfo=UTC)

    assert scheduler.schedule_due(now) == 1
    assert scheduler.schedule_due(now) == 0
    page = CollectionService(database, sources, evidence_configured=True).runs(20, None)
    assert len(page.items) == 1 and page.next_cursor is None
    run = page.items[0]
    assert run.trigger == "scheduled"
    assert run.schedule_slot == datetime(2026, 9, 15, tzinfo=UTC)
    assert run.window_since == datetime(2026, 9, 14, tzinfo=UTC)
    assert run.window_until == datetime(2026, 9, 15, tzinfo=UTC)
    assert run.reserved_requests == 1

    MonitorService(database, sources).change_state(
        monitor.id,
        MonitorStateChange(expected_version=monitor.current_version),
        "paused",
    )
    assert scheduler.schedule_due(datetime(2026, 9, 16, 12, 34, tzinfo=UTC)) == 0


def test_scheduler_uses_the_same_batch_orchestration_for_all_queries(database):
    sources = AdmittedSources()
    active_monitor(database, daily_requests=100, aliases=["人工智能"])
    scheduler = CollectionScheduler(database, sources, evidence_configured=True)
    now = datetime(2026, 9, 15, 12, 34, tzinfo=UTC)

    assert scheduler.schedule_due(now) == 2
    assert scheduler.schedule_due(now) == 0
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 2
        usage = session.scalar(select(CollectionBudgetUsage))
        assert usage is not None and usage.reserved_requests == 2


def test_scheduler_without_evidence_configuration_writes_nothing(database):
    sources = AdmittedSources()
    active_monitor(database)
    scheduler = CollectionScheduler(database, sources, evidence_configured=False)
    assert scheduler.schedule_due(datetime(2026, 9, 15, 12, 34, tzinfo=UTC)) == 0
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 0


def test_scheduler_skips_active_monitor_when_source_is_not_eligible(database):
    active_monitor(database)
    scheduler = CollectionScheduler(database, SourceService(), evidence_configured=True)
    assert scheduler.schedule_due(datetime(2026, 9, 15, 12, 34, tzinfo=UTC)) == 0
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 0


def test_run_list_uses_stable_keyset_cursor(database):
    sources = AdmittedSources()
    monitor = active_monitor(database, daily_requests=4)
    service = CollectionService(database, sources, evidence_configured=True)
    first_created = service.create_run(manual_run(monitor, "run-page-1"))
    second_created = service.create_run(manual_run(monitor, "run-page-2"))

    first_page = service.runs(1, None)
    assert len(first_page.items) == 1
    assert first_page.next_cursor is not None
    second_page = service.runs(1, first_page.next_cursor)
    assert len(second_page.items) == 1
    assert second_page.next_cursor is None
    assert {first_page.items[0].id, second_page.items[0].id} == {
        first_created.id,
        second_created.id,
    }

    with pytest.raises(AppError) as error:
        service.runs(1, "not-a-cursor")
    assert error.value.code == "invalid_cursor"
