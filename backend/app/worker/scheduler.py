from __future__ import annotations

import signal
from collections.abc import Callable, Iterable, Sized
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from importlib import import_module
from threading import Event
from typing import Protocol, cast
from uuid import UUID, uuid5

import structlog
from sqlalchemy.orm import Session, sessionmaker

from analysis.services import AnalysisService
from connections.schemas import SourceEntryPoint
from connections.services import load_applied_source_presets_in_transaction
from content.discovery import plan_scheduled_keyword_discovery
from content.schemas import KeywordDiscoveryRunInput
from content.services import CommentScanService
from core.config import get_settings
from core.logging import configure_logging

# The scheduler is its own process; import the canonical registry to resolve ORM foreign keys.
from db.metadata import metadata as _registered_metadata  # noqa: F401
from db.session import create_db_engine, create_session_factory
from jobs.services import JobService, load_job_execution_configuration
from knowledge.services import KnowledgeExportService
from monitors.services import DueCollectionSchedule, MonitorScheduleService
from notifications.services import NotificationService

SCHEDULER_POLL_SECONDS = 30
COLLECTION_OPERATION_NAMESPACE = UUID("515944a7-070b-4b27-86a4-bc811109031d")
_COLLECTION_PAGE_SIZE = 100
_COLLECTION_MAX_PAGES = 3
_COLLECTION_MAX_REQUESTS = 3
_COLLECTION_MAX_SECONDS = 90

SchedulerScanFunction = Callable[[Session, datetime], int]


@dataclass(frozen=True, slots=True)
class SchedulerScan:
    name: str
    run_in_transaction: SchedulerScanFunction


class _ReportScanService(Protocol):
    def enqueue_due_in_transaction(self, *, now: datetime) -> int | Sized: ...


def _utc_text(value: datetime) -> str:
    if value.tzinfo is None:
        raise ValueError("scheduler timestamps must be timezone-aware")
    return value.astimezone(UTC).isoformat().replace("+00:00", "Z")


def collection_schedule_id(schedule: DueCollectionSchedule) -> UUID:
    identity = ":".join(
        (
            "schedule",
            str(schedule.owner_id),
            str(schedule.topic_id),
            schedule.source_key,
            schedule.capability.value,
        )
    )
    return uuid5(COLLECTION_OPERATION_NAMESPACE, identity)


def collection_operation_id(
    schedule: DueCollectionSchedule,
    window_start: datetime,
) -> UUID:
    identity = f"collect:{collection_schedule_id(schedule)}:{_utc_text(window_start)}"
    return uuid5(COLLECTION_OPERATION_NAMESPACE, identity)


def collection_window_start(
    now: datetime,
    *,
    interval_seconds: int,
    previous_end: datetime | None,
    lookback_seconds: int = 0,
) -> datetime:
    """Start where the last window ended, reaching back to catch late-indexed posts.

    Sources publish and index with delays, so a window judged by publish time must
    overlap earlier ones; re-seen posts are deduplicated when they are saved.
    """
    if now.tzinfo is None or not 600 <= interval_seconds <= 86_400:
        raise ValueError("collection window requires an aware time and valid interval")
    if lookback_seconds < 0:
        raise ValueError("collection lookback cannot be negative")
    now_utc = now.astimezone(UTC)
    if previous_end is None:
        base = now_utc - timedelta(seconds=interval_seconds)
    else:
        if previous_end.tzinfo is None:
            raise ValueError("previous collection window end must be timezone-aware")
        base = previous_end.astimezone(UTC)
        if base >= now_utc:
            raise ValueError("previous collection window end must precede the current scan")
    return base - timedelta(seconds=lookback_seconds)


def _previous_collection_end(
    session: Session,
    *,
    schedule: DueCollectionSchedule,
) -> datetime | None:
    if schedule.last_job_id is None:
        return None
    configuration = load_job_execution_configuration(session, job_id=schedule.last_job_id)
    if (
        configuration is None
        or configuration.owner_id != schedule.owner_id
        or configuration.kind != "keyword.search"
        or configuration.observation.configuration_ref != f"topic:{schedule.topic_id}"
        or configuration.observation.source_key != schedule.source_key
    ):
        raise RuntimeError("last collection job does not match the claimed schedule")
    value = configuration.scope.get("ends_at")
    if not isinstance(value, str):
        raise RuntimeError("last collection job has no window end")
    try:
        end = datetime.fromisoformat(value)
    except ValueError as error:
        raise RuntimeError("last collection job window end is invalid") from error
    if end.utcoffset() != timedelta(0):
        raise RuntimeError("last collection job window end must be UTC")
    return end


def _accept_collection_schedule(
    session: Session,
    *,
    schedule: DueCollectionSchedule,
    now: datetime,
) -> tuple[UUID, ...]:
    applied = load_applied_source_presets_in_transaction(
        session,
        owner_id=schedule.owner_id,
        source_keys=(schedule.source_key,),
    ).get(schedule.source_key)
    if applied is None or schedule.capability not in applied.capabilities:
        raise RuntimeError("collection schedule source preset is not currently applied")

    window_start = collection_window_start(
        now,
        interval_seconds=schedule.interval_seconds,
        previous_end=_previous_collection_end(session, schedule=schedule),
        lookback_seconds=get_settings().collection_lookback_seconds,
    )
    schedule_operation_id = collection_operation_id(schedule, window_start)
    accepted_ids: list[UUID] = []
    for query in schedule.search_queries:
        operation_id = uuid5(schedule_operation_id, f"query:{query}")
        command = plan_scheduled_keyword_discovery(
            KeywordDiscoveryRunInput(
                run_id=operation_id,
                configuration_ref=f"topic:{schedule.topic_id}",
                configuration_version=schedule.topic_version,
                source_key=schedule.source_key,
                connection_id=applied.connection_id,
                connection_version=applied.connection_version,
                primary_query=query,
                starts_at=window_start,
                ends_at=now,
                page_size=_COLLECTION_PAGE_SIZE,
                latest_max_pages=_COLLECTION_MAX_PAGES,
                latest_max_requests=_COLLECTION_MAX_REQUESTS,
                top_max_pages=1,
                top_max_requests=1,
                max_seconds=_COLLECTION_MAX_SECONDS,
                entry_point=SourceEntryPoint.SCHEDULED,
                scheduled_for_at=now,
            )
        )
        accepted = JobService(session, clock=lambda: now).accept_in_transaction(
            owner_id=schedule.owner_id,
            command=command,
        )
        accepted_ids.append(accepted.id)
    if not accepted_ids:
        raise RuntimeError("collection schedule has no upstream search queries")
    MonitorScheduleService(session).advance_collection_in_transaction(
        schedule=schedule,
        job_id=accepted_ids[-1],
        next_run_at=now + timedelta(seconds=schedule.interval_seconds),
        updated_at=now,
    )
    return tuple(accepted_ids)


def enqueue_due_collections_in_transaction(session: Session, now: datetime) -> int:
    """Claim and accept due collection rows; isolate one bad row with a savepoint."""
    if not session.in_transaction():
        raise RuntimeError("collection scan requires the caller's transaction")
    now_utc = now.astimezone(UTC) if now.tzinfo is not None else now
    schedules = MonitorScheduleService(session).claim_due_collections_in_transaction(now=now_utc)
    accepted = 0
    logger = structlog.get_logger("scheduler")
    for schedule in schedules:
        try:
            with session.begin_nested():
                accepted_ids = _accept_collection_schedule(session, schedule=schedule, now=now_utc)
        except Exception as error:
            logger.warning(
                "scheduler_collection_row_failed",
                owner_id=str(schedule.owner_id),
                topic_id=str(schedule.topic_id),
                source_key=schedule.source_key,
                error_type=type(error).__name__,
                exc_info=True,
            )
            continue
        accepted += len(accepted_ids)
    return accepted


def enqueue_due_comments_in_transaction(session: Session, now: datetime) -> int:
    return CommentScanService(session).enqueue_due_comments_in_transaction(now=now)


def enqueue_due_analysis_in_transaction(session: Session, now: datetime) -> int:
    if not session.in_transaction():
        raise RuntimeError("analysis scan requires the caller's transaction")
    if now.tzinfo is None:
        raise ValueError("analysis scan time must be timezone-aware")
    topics = MonitorScheduleService(session).list_active_topics_for_scanning_in_transaction()
    accepted = 0
    logger = structlog.get_logger("scheduler")
    for topic in topics:
        try:
            with session.begin_nested():
                jobs = AnalysisService(session).enqueue_due_batches_in_transaction(
                    owner_id=topic.owner_id,
                    topic_id=topic.topic_id,
                    now=now.astimezone(UTC),
                )
        except Exception as error:
            logger.warning(
                "scheduler_analysis_topic_failed",
                owner_id=str(topic.owner_id),
                topic_id=str(topic.topic_id),
                error_type=type(error).__name__,
                exc_info=True,
            )
            continue
        accepted += len(jobs)
    return accepted


def _optional_report_scan() -> SchedulerScan | None:
    try:
        module = import_module("reports.services")
    except ModuleNotFoundError as error:
        if error.name in {"reports", "reports.services"}:
            return None
        raise
    service_type = getattr(module, "ReportService", None)
    method = getattr(service_type, "enqueue_due_in_transaction", None)
    if service_type is None or not callable(method):
        return None
    factory = cast(Callable[[Session], _ReportScanService], service_type)

    def run_report_scan(session: Session, now: datetime) -> int:
        result = factory(session).enqueue_due_in_transaction(now=now)
        return result if isinstance(result, int) else len(result)

    return SchedulerScan(name="reports", run_in_transaction=run_report_scan)


def _registered_scheduler_scans() -> tuple[SchedulerScan, ...]:
    scans = [
        SchedulerScan(
            name="collection",
            run_in_transaction=enqueue_due_collections_in_transaction,
        ),
        SchedulerScan(
            name="comments",
            run_in_transaction=enqueue_due_comments_in_transaction,
        ),
        SchedulerScan(
            name="analysis",
            run_in_transaction=enqueue_due_analysis_in_transaction,
        ),
    ]
    report_scan = _optional_report_scan()
    if report_scan is not None:
        scans.append(report_scan)
    scans.append(
        SchedulerScan(
            name="knowledge",
            run_in_transaction=lambda session, now: KnowledgeExportService(
                session, get_settings()
            ).enqueue_due_in_transaction(now=now),
        )
    )
    scans.append(
        SchedulerScan(
            name="notifications",
            run_in_transaction=lambda session, now: NotificationService(
                session, get_settings()
            ).enqueue_due_in_transaction(now=now),
        )
    )
    return tuple(scans)


def run_scheduler_round(
    sessions: sessionmaker[Session],
    scans: Iterable[SchedulerScan],
    *,
    now: datetime,
) -> dict[str, int]:
    logger = structlog.get_logger("scheduler")
    results: dict[str, int] = {}
    for scan in scans:
        try:
            with sessions() as session, session.begin():
                results[scan.name] = scan.run_in_transaction(session, now)
        except Exception as error:
            logger.error(
                "scheduler_scan_failed",
                scan=scan.name,
                error_type=type(error).__name__,
                exc_info=True,
            )
    return results


def run_scheduler() -> None:
    settings = get_settings()
    configure_logging(settings.log_level)
    logger = structlog.get_logger("scheduler")
    stopping = Event()

    def request_stop(_signum: int, _frame: object) -> None:
        stopping.set()

    signal.signal(signal.SIGINT, request_stop)
    signal.signal(signal.SIGTERM, request_stop)

    engine = create_db_engine(settings)
    sessions = create_session_factory(engine)
    scans = _registered_scheduler_scans()
    try:
        while not stopping.is_set():
            started_at = datetime.now(UTC)
            results = run_scheduler_round(sessions, scans, now=started_at)
            logger.info("scheduler_round_completed", scan_counts=results)
            stopping.wait(SCHEDULER_POLL_SECONDS)
    finally:
        engine.dispose()
    logger.info("scheduler_stopped")


if __name__ == "__main__":
    run_scheduler()
