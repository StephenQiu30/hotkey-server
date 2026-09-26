from datetime import UTC, datetime, timedelta
from uuid import uuid4, uuid5

from sqlalchemy.dialects import postgresql

from content.services import (
    COMMENT_OPERATION_NAMESPACE,
    comment_bucket_start,
    comment_operation_id,
)
from monitors.services import (
    DueCollectionSchedule,
    MonitorScheduleService,
    NormalizedMonitorRules,
    scheduled_collection_queries,
)
from sources.contracts import SourceCapability
from worker.scheduler import (
    COLLECTION_OPERATION_NAMESPACE,
    SchedulerScan,
    _registered_scheduler_scans,
    collection_operation_id,
    collection_schedule_id,
    collection_window_start,
    run_scheduler_round,
)


def _schedule() -> DueCollectionSchedule:
    return DueCollectionSchedule(
        owner_id=uuid4(),
        topic_id=uuid4(),
        topic_version=3,
        search_queries=("产品", "故障"),
        source_key="hackernews",
        capability=SourceCapability.SEARCH,
        interval_seconds=600,
        next_run_at=datetime(2026, 9, 25, 0, tzinfo=UTC),
        last_job_id=None,
    )


def test_collection_operation_id_is_stable_for_schedule_and_window_start() -> None:
    schedule = _schedule()
    start = datetime(2026, 9, 25, 0, tzinfo=UTC)
    expected_schedule_id = uuid5(
        COLLECTION_OPERATION_NAMESPACE,
        ":".join(
            (
                "schedule",
                str(schedule.owner_id),
                str(schedule.topic_id),
                schedule.source_key,
                schedule.capability.value,
            )
        ),
    )
    expected_operation_id = uuid5(
        COLLECTION_OPERATION_NAMESPACE,
        f"collect:{expected_schedule_id}:2026-09-25T00:00:00Z",
    )

    assert collection_schedule_id(schedule) == expected_schedule_id
    assert collection_operation_id(schedule, start) == expected_operation_id
    assert collection_operation_id(schedule, start) != collection_operation_id(
        schedule, start + timedelta(minutes=10)
    )


def test_collection_operation_id_changes_with_schedule_identity() -> None:
    schedule = _schedule()
    changed = DueCollectionSchedule(
        owner_id=schedule.owner_id,
        topic_id=schedule.topic_id,
        topic_version=schedule.topic_version,
        search_queries=schedule.search_queries,
        source_key="google_news",
        capability=schedule.capability,
        interval_seconds=schedule.interval_seconds,
        next_run_at=schedule.next_run_at,
        last_job_id=schedule.last_job_id,
    )
    start = datetime(2026, 9, 25, 0, tzinfo=UTC)

    assert collection_operation_id(schedule, start) != collection_operation_id(changed, start)


def test_scheduled_collection_uses_each_match_any_term_and_falls_back_to_joined_match_all() -> None:
    assert scheduled_collection_queries(
        NormalizedMonitorRules(
            match_any=("产品", "故障"),
            match_all=("中国",),
            exclude=(),
        )
    ) == ("产品", "故障")
    assert scheduled_collection_queries(
        NormalizedMonitorRules(
            match_any=(),
            match_all=("产品", "故障"),
            exclude=(),
        )
    ) == ("产品 故障",)


def test_comment_operation_id_uses_six_hour_utc_bucket() -> None:
    content_id = uuid4()
    now = datetime(2026, 9, 25, 11, 59, 59, tzinfo=UTC)
    bucket = datetime(2026, 9, 25, 6, tzinfo=UTC)

    assert comment_bucket_start(now) == bucket
    assert comment_operation_id(content_id, bucket) == uuid5(
        COMMENT_OPERATION_NAMESPACE,
        f"comments:{content_id}:2026-09-25T06:00:00Z",
    )


def test_scheduler_registers_collection_comments_and_analysis_scans() -> None:
    names = tuple(scan.name for scan in _registered_scheduler_scans())

    assert names == (
        "collection",
        "comments",
        "analysis",
        "reports",
        "knowledge",
    )


def test_collection_window_starts_at_previous_end_or_one_interval_before_now() -> None:
    now = datetime(2026, 9, 25, 1, tzinfo=UTC)
    previous_end = datetime(2026, 9, 25, 0, 45, tzinfo=UTC)

    assert collection_window_start(now, interval_seconds=600, previous_end=None) == now - timedelta(
        minutes=10
    )
    assert (
        collection_window_start(now, interval_seconds=600, previous_end=previous_end)
        == previous_end
    )


def test_collection_claim_uses_skip_locked() -> None:
    class EmptyRows:
        @staticmethod
        def all() -> list[object]:
            return []

    class CapturingSession:
        statement: object | None = None

        @staticmethod
        def in_transaction() -> bool:
            return True

        def execute(self, statement: object) -> EmptyRows:
            self.statement = statement
            return EmptyRows()

    session = CapturingSession()
    service = MonitorScheduleService(session)  # type: ignore[arg-type]

    assert (
        service.claim_due_collections_in_transaction(now=datetime(2026, 9, 25, 0, tzinfo=UTC)) == ()
    )
    assert session.statement is not None
    sql = str(session.statement.compile(dialect=postgresql.dialect()))  # type: ignore[union-attr]
    assert "FOR UPDATE OF monitor_schedules SKIP LOCKED" in sql


def test_each_registered_scan_uses_an_independent_session_and_failure_does_not_block_next() -> None:
    class Context:
        def __enter__(self) -> "Context":
            return self

        def __exit__(self, *_args: object) -> None:
            return None

    class FakeSession(Context):
        @staticmethod
        def begin() -> "Context":
            return Context()

    created: list[FakeSession] = []

    def sessions() -> FakeSession:
        session = FakeSession()
        created.append(session)
        return session

    seen: list[FakeSession] = []

    def fail(session: object, _now: datetime) -> int:
        seen.append(session)  # type: ignore[arg-type]
        raise RuntimeError("isolated failure")

    def succeed(session: object, _now: datetime) -> int:
        seen.append(session)  # type: ignore[arg-type]
        return 2

    results = run_scheduler_round(
        sessions,  # type: ignore[arg-type]
        (
            SchedulerScan(name="failed", run_in_transaction=fail),  # type: ignore[arg-type]
            SchedulerScan(name="succeeded", run_in_transaction=succeed),  # type: ignore[arg-type]
        ),
        now=datetime(2026, 9, 25, 0, tzinfo=UTC),
    )

    assert results == {"succeeded": 2}
    assert seen == created
    assert len({id(session) for session in seen}) == 2
