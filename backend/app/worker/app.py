from __future__ import annotations

import os
import signal
import socket
import time
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from threading import Event
from typing import NoReturn
from uuid import UUID

import structlog
from confluent_kafka import Message
from sqlalchemy.orm import Session, sessionmaker
from structlog.contextvars import bound_contextvars

from ai.services import recover_abandoned_ai_calls_in_transaction
from analysis.editorial_services import EditorialExecutor
from analysis.evaluation_execution import SelectBenchExecutor
from analysis.services import AnalysisAnnotateExecutor
from analysis.translation_services import ContentTranslationExecutor
from content.collection import (
    WebPageCollectionExecutor,
    recover_webpage_collection_usage_in_transaction,
)
from content.collection_cancellation import recover_interrupted_collection_in_transaction
from content.comments_execution import CommentsExecutor
from content.discovery_execution import KeywordDiscoveryExecutor
from content.hotlist import recover_hotlist_usage_in_transaction
from content.hotlist_execution import HotlistExecutor
from core.config import (
    JOB_PROCESS_STARTUP_TIMEOUT_SECONDS,
    JOB_PROCESS_TERMINATE_GRACE_SECONDS,
    Settings,
    get_settings,
)
from core.logging import configure_logging

# Spawn starts a fresh interpreter; import the canonical registry to resolve ORM foreign keys.
from db.metadata import metadata as _registered_metadata  # noqa: F401
from db.session import create_db_engine, create_session_factory
from events.consolidation import EventConsolidationExecutor
from events.digest import EventDigestExecutor
from events.embedding_execution import EventEmbeddingExecutor
from events.heat import EventHeatExecutor
from events.services import EventClusterExecutor
from events.signals import EventSignalExecutor
from evidence.adapters.media_storage import create_media_storage
from jobs.execution import (
    CheckpointValue,
    Clock,
    ExecutionLease,
    JobCompletion,
    JobExecutionError,
    JobExecutionFailure,
    JobExecutionService,
    JobLeaseUnavailableError,
    JobProgress,
    MessageReference,
)
from jobs.schemas import JobFailureCategory, JobMessage, JobStage, JobStatus
from jobs.services import JOB_ACCEPTED_TOPIC, OutboxService, load_job_execution_configuration
from knowledge.services import KnowledgeExportExecutor
from leaderboard.execution import LeaderboardRefreshExecutor
from monitors.codex_job import CodexResetJobExecutor
from notifications.admission import enqueue_codex_notifications_in_transaction
from notifications.executor import NotificationExecutor
from notifications.scan import NotificationScanExecutor
from notifications.services import mark_interrupted_sending_in_transaction
from operations.heartbeat import ProcessHeartbeatReporter
from operations.indexnow_services import IndexNowExecutor
from operations.maintenance import OperationsMaintenanceExecutor
from publication.execution import PublicationRepublishExecutor
from publication.media_mirror_execution import MediaObjectStorage, PublicationMediaExecutor
from reports.edition_services import EditionExecutor
from reports.export_execution import PrivateExportExecutor
from reports.services import DailyReportExecutor
from sources.adapters.firecrawl import FirecrawlAdapter
from sources.editorial_group_job import EditorialXGroupJobExecutor
from sources.editorial_job import EditorialSourceJobExecutor
from sources.editorial_preview_job import EditorialSourcePreviewExecutor
from sources.icons_job import SourceIconJobExecutor
from worker.execution import (
    IsolatedProcessResult,
    JobProcessChildError,
    JobProcessCrashedError,
    JobProcessOutcome,
    JobProcessShutdownError,
    JobProcessSupervisor,
)
from worker.messaging import (
    MessageDeferredError,
    MessageHandler,
    StopProcessingMessageError,
    create_producer,
    decode_job_message,
    publish_outbox,
    run_consumer_loop,
)

JobHandler = Callable[["JobExecutionContext"], JobCompletion | None]


@dataclass(slots=True)
class JobExecutionContext:
    message: JobMessage
    lease: ExecutionLease
    _sessions: sessionmaker[Session]
    _lease_seconds: int
    _clock: Clock | None

    def save_checkpoint(
        self,
        sequence: int,
        checkpoint: Mapping[str, CheckpointValue],
        *,
        progress: JobProgress | None = None,
    ) -> None:
        with self._sessions() as session:
            self.lease = JobExecutionService(
                session,
                lease_seconds=self._lease_seconds,
                clock=self._clock,
            ).save_checkpoint(
                self.lease,
                sequence=sequence,
                checkpoint=checkpoint,
                progress=progress,
            )

    def cancellation_requested(self) -> bool:
        with self._sessions() as session:
            return JobExecutionService(
                session,
                lease_seconds=self._lease_seconds,
                clock=self._clock,
            ).cancellation_requested(self.lease)

    def begin_request(self) -> bool:
        with self._sessions() as session:
            self.lease, allowed = JobExecutionService(
                session,
                lease_seconds=self._lease_seconds,
                clock=self._clock,
            ).begin_request(self.lease)
        return allowed


@dataclass(frozen=True, slots=True)
class ChildJobFailure:
    error_code: str
    category: str
    occurred_at: datetime
    next_action: str
    manual_retry_allowed: bool
    retry_at: datetime | None
    max_attempts: int | None

    @classmethod
    def from_failure(cls, failure: JobExecutionFailure) -> ChildJobFailure:
        return cls(
            error_code=failure.error_code,
            category=failure.category.value,
            occurred_at=failure.occurred_at,
            next_action=failure.next_action,
            manual_retry_allowed=failure.manual_retry_allowed,
            retry_at=failure.retry_at,
            max_attempts=failure.max_attempts,
        )

    def to_failure(self) -> JobExecutionFailure:
        return JobExecutionFailure(
            error_code=self.error_code,
            category=JobFailureCategory(self.category),
            occurred_at=self.occurred_at,
            next_action=self.next_action,
            manual_retry_allowed=self.manual_retry_allowed,
            retry_at=self.retry_at,
            max_attempts=self.max_attempts,
        )


@dataclass(frozen=True, slots=True)
class ChildJobCompletion:
    status: str
    failure: ChildJobFailure | None = None

    @classmethod
    def from_completion(cls, completion: JobCompletion) -> ChildJobCompletion:
        return cls(
            status=completion.status.value,
            failure=(
                ChildJobFailure.from_failure(completion.failure)
                if completion.failure is not None
                else None
            ),
        )

    def to_completion(self) -> JobCompletion:
        return JobCompletion(
            status=JobStatus(self.status),
            failure=self.failure.to_failure() if self.failure is not None else None,
        )


@dataclass(frozen=True, slots=True)
class ChildJobResult:
    lease: ExecutionLease
    completion: ChildJobCompletion | None = None
    failure: ChildJobFailure | None = None

    def __post_init__(self) -> None:
        if self.completion is not None and self.failure is not None:
            raise ValueError("child result cannot contain both completion and failure")


def _run_job_in_child(
    message: JobMessage,
    lease: ExecutionLease,
    lease_seconds: int,
) -> ChildJobResult:
    settings = get_settings()
    engine = create_db_engine(settings)
    sessions = create_session_factory(engine, settings=settings)
    media_storage = create_media_storage(settings)
    try:
        handler = _registered_job_handlers(sessions, settings, media_storage=media_storage).get(
            message.kind
        )
        if handler is None:
            occurred_at = datetime.now(UTC)
            return ChildJobResult(
                lease=lease,
                failure=ChildJobFailure.from_failure(
                    JobExecutionFailure(
                        error_code="job_handler_unavailable",
                        category=JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                        occurred_at=occurred_at,
                        next_action="启用匹配当前任务类型的处理器后重试",
                        manual_retry_allowed=True,
                    )
                ),
            )

        context = JobExecutionContext(
            message=message,
            lease=lease,
            _sessions=sessions,
            _lease_seconds=lease_seconds,
            _clock=None,
        )
        try:
            completion = handler(context)
        except JobExecutionFailure as failure:
            return ChildJobResult(
                lease=context.lease,
                failure=ChildJobFailure.from_failure(failure),
            )
        return ChildJobResult(
            lease=context.lease,
            completion=(
                ChildJobCompletion.from_completion(completion) if completion is not None else None
            ),
        )
    finally:
        if media_storage is not None:
            media_storage.close()
        engine.dispose()


def create_job_message_handler(
    sessions: sessionmaker[Session],
    handlers: Mapping[str, JobHandler],
    *,
    worker_id: str,
    lease_seconds: int,
    clock: Clock | None = None,
    supervisor: JobProcessSupervisor | None = None,
    stopping: Event | None = None,
    job_execution_timeout_seconds: Callable[[str, str | None], float] | None = None,
) -> MessageHandler:
    dispatch = create_job_dispatcher(
        sessions,
        handlers,
        worker_id=worker_id,
        lease_seconds=lease_seconds,
        clock=clock,
        supervisor=supervisor,
        stopping=stopping,
        job_execution_timeout_seconds=job_execution_timeout_seconds,
    )

    def handle(message: Message) -> None:
        body, reference = decode_job_message(message)
        dispatch(body, reference)

    return handle


def create_job_dispatcher(
    sessions: sessionmaker[Session],
    handlers: Mapping[str, JobHandler],
    *,
    worker_id: str,
    lease_seconds: int,
    clock: Clock | None = None,
    supervisor: JobProcessSupervisor | None = None,
    stopping: Event | None = None,
    job_execution_timeout_seconds: Callable[[str, str | None], float] | None = None,
) -> Callable[[JobMessage, MessageReference], None]:
    def handle(body: JobMessage, reference: MessageReference) -> None:
        log_context: dict[str, str | int] = {
            "operation_id": str(body.operation_id),
            "job_id": str(body.job_id),
            "job_kind": body.kind,
            "configuration_version": body.configuration_version,
        }
        if body.source_key is not None and body.source_capability is not None:
            log_context["source_key"] = body.source_key
            log_context["source_capability"] = body.source_capability.value

        with bound_contextvars(**log_context):
            with sessions() as session:
                execution = JobExecutionService(
                    session,
                    lease_seconds=lease_seconds,
                    clock=clock,
                )
                if execution.is_processed(body.message_id):
                    return
                if execution.acknowledge_cancelled(
                    job_id=body.job_id,
                    message=reference,
                ):
                    return
                try:
                    lease = execution.acquire(job_id=body.job_id, worker_id=worker_id)
                except JobLeaseUnavailableError as error:
                    retry_at = execution.current_lease_expiration(body.job_id)
                    if retry_at is None:
                        raise
                    raise MessageDeferredError(retry_at) from error
                if body.kind in {
                    "analysis.annotate",
                    "analysis.editorial",
                    "analysis.translate",
                    "analysis.selectbench",
                    "events.cluster",
                    "events.digest",
                    "events.consolidate",
                    "events.signals",
                    "events.embed",
                    "report.daily",
                    "report.weekly",
                    "report.edition",
                    "monitor.codex_reset.tick",
                }:
                    with session.begin():
                        execution.require_current_operation_in_transaction(
                            lease, owner_id=body.owner_id, operation_id=body.operation_id
                        )
                        recover_abandoned_ai_calls_in_transaction(
                            session,
                            owner_id=body.owner_id,
                            job_id=body.job_id,
                            current_epoch=lease.epoch,
                            now=_now(clock),
                        )
                handler = handlers.get(body.kind)
                if handler is None:
                    occurred_at = clock() if clock is not None else datetime.now(UTC)
                    execution.record_failure(
                        lease,
                        message=reference,
                        failure=JobExecutionFailure(
                            error_code="job_handler_unavailable",
                            category=JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                            occurred_at=occurred_at,
                            next_action="启用匹配当前任务类型的处理器后重试",
                            manual_retry_allowed=True,
                        ),
                    )
                    return

            if supervisor is not None:
                if stopping is None:
                    raise ValueError("a stopping event is required for supervised handlers")
                next_heartbeat = time.monotonic() + max(1.0, lease_seconds / 3)

                def watch_lease() -> bool:
                    nonlocal next_heartbeat
                    if time.monotonic() >= next_heartbeat:
                        try:
                            with sessions() as heartbeat_session:
                                _, cancelled = JobExecutionService(
                                    heartbeat_session, lease_seconds=lease_seconds, clock=clock
                                ).heartbeat(lease)
                        except JobExecutionError as error:
                            raise JobProcessCrashedError(type(error).__name__) from None
                        next_heartbeat = time.monotonic() + max(1.0, lease_seconds / 3)
                        return cancelled
                    return _cancellation_requested(
                        sessions, lease=lease, lease_seconds=lease_seconds, clock=clock
                    )

                try:
                    result = supervisor.run(
                        _run_job_in_child,
                        (body, lease, lease_seconds),
                        cancellation_requested=watch_lease,
                        stopping=stopping,
                        execution_timeout_seconds=(
                            job_execution_timeout_seconds(body.kind, body.source_key)
                            if job_execution_timeout_seconds is not None
                            else None
                        ),
                    )
                except JobProcessShutdownError as error:
                    if body.kind == "notification.send":
                        _mark_interrupted_notification(sessions, message=body, now=_now(clock))
                    raise StopProcessingMessageError from error
                except JobProcessChildError as error:
                    failure = _child_exception_failure(error, occurred_at=_now(clock))
                    _finalize_supervised_result(
                        sessions,
                        message=body,
                        reference=reference,
                        report=ChildJobResult(
                            lease=lease,
                            failure=ChildJobFailure.from_failure(failure),
                        ),
                        lease_seconds=lease_seconds,
                        clock=clock,
                    )
                    structlog.get_logger("worker").error(
                        "job_child_failed",
                        error_code=failure.error_code,
                        exception_type=error.exception_type,
                        failure_category=failure.category.value,
                        **(
                            {"error_message": error.error_message[:200]}
                            if error.exception_type in {"ValueError", "TypeError"}
                            and error.error_message is not None
                            else {}
                        ),
                    )
                    return
                except JobProcessCrashedError as error:
                    _defer_after_child_exit(
                        sessions,
                        lease=lease,
                        lease_seconds=lease_seconds,
                        clock=clock,
                        cause=error,
                        message=body,
                    )

                if result.outcome is JobProcessOutcome.TIMED_OUT:
                    report = ChildJobResult(
                        lease=lease,
                        failure=ChildJobFailure.from_failure(
                            JobExecutionFailure(
                                error_code="job_execution_timeout",
                                category=JobFailureCategory.TRANSIENT,
                                occurred_at=_now(clock),
                                next_action="确认目标服务状态后手动重试",
                                manual_retry_allowed=True,
                            )
                        ),
                    )
                else:
                    if result.outcome is JobProcessOutcome.CANCELLED:
                        report = ChildJobResult(lease=lease)
                    else:
                        try:
                            report = _validate_child_result(result, lease)
                        except JobProcessCrashedError as error:
                            _defer_after_child_exit(
                                sessions,
                                lease=lease,
                                lease_seconds=lease_seconds,
                                clock=clock,
                                cause=error,
                                message=body,
                            )
                _finalize_supervised_result(
                    sessions,
                    message=body,
                    reference=reference,
                    report=report,
                    lease_seconds=lease_seconds,
                    clock=clock,
                )
                return

            context = JobExecutionContext(
                message=body,
                lease=lease,
                _sessions=sessions,
                _lease_seconds=lease_seconds,
                _clock=clock,
            )
            try:
                completion = handler(context)
            except JobExecutionFailure as failure:
                with sessions() as session:
                    JobExecutionService(
                        session,
                        lease_seconds=lease_seconds,
                        clock=clock,
                    ).record_failure(
                        context.lease,
                        message=reference,
                        failure=failure,
                    )
                return
            with sessions() as session:
                JobExecutionService(
                    session,
                    lease_seconds=lease_seconds,
                    clock=clock,
                ).complete(
                    context.lease,
                    message=reference,
                    completion=completion,
                )

    return handle


def _now(clock: Clock | None) -> datetime:
    return clock() if clock is not None else datetime.now(UTC)


def _child_exception_failure(
    error: JobProcessChildError, *, occurred_at: datetime
) -> JobExecutionFailure:
    if error.error_code == "parse_error":
        category = JobFailureCategory.PARSE_ERROR
        next_action = "检查数据格式后手动重试"
    elif error.error_code == "invalid_response":
        category = JobFailureCategory.INVALID_RESPONSE
        next_action = "检查数据契约后手动重试"
    elif error.error_code == "network_error":
        return JobExecutionFailure(
            error_code=error.error_code,
            category=JobFailureCategory.TRANSIENT,
            occurred_at=occurred_at,
            next_action="等待外部服务恢复后自动重试",
            manual_retry_allowed=True,
            retry_at=occurred_at + timedelta(seconds=30),
            max_attempts=3,
        )
    else:
        category = JobFailureCategory.CONFIGURATION_UNAVAILABLE
        next_action = "检查任务执行异常后手动重试"
    return JobExecutionFailure(
        error_code=error.error_code,
        category=category,
        occurred_at=occurred_at,
        next_action=next_action,
        manual_retry_allowed=True,
    )


def _cancellation_requested(
    sessions: sessionmaker[Session],
    *,
    lease: ExecutionLease,
    lease_seconds: int,
    clock: Clock | None,
) -> bool:
    try:
        with sessions() as session:
            return JobExecutionService(
                session,
                lease_seconds=lease_seconds,
                clock=clock,
            ).cancellation_requested(lease)
    except JobExecutionError as error:
        raise JobProcessCrashedError(type(error).__name__) from None


def _lease_retry_time(
    sessions: sessionmaker[Session],
    *,
    lease: ExecutionLease,
    lease_seconds: int,
    clock: Clock | None,
) -> datetime:
    with sessions() as session:
        expires_at = JobExecutionService(
            session,
            lease_seconds=lease_seconds,
            clock=clock,
        ).current_lease_expiration(lease.job_id)
    if expires_at is not None:
        return expires_at
    return _now(clock) + timedelta(seconds=1)


def _defer_after_child_exit(
    sessions: sessionmaker[Session],
    *,
    lease: ExecutionLease,
    lease_seconds: int,
    clock: Clock | None,
    cause: JobProcessCrashedError,
    message: JobMessage,
) -> NoReturn:
    if message.kind == "notification.send":
        _mark_interrupted_notification(sessions, message=message, now=_now(clock))
    if message.kind == "source.hotlist" and message.source_key is not None:
        with sessions() as session, session.begin():
            recover_hotlist_usage_in_transaction(
                session,
                owner_id=message.owner_id,
                operation_id=message.operation_id,
                source_key=message.source_key,
                finished_at=_now(clock),
            )
    raise MessageDeferredError(
        _lease_retry_time(
            sessions,
            lease=lease,
            lease_seconds=lease_seconds,
            clock=clock,
        )
    ) from cause


def _mark_interrupted_notification(
    sessions: sessionmaker[Session], *, message: JobMessage, now: datetime
) -> None:
    with sessions() as session:
        configuration = load_job_execution_configuration(session, job_id=message.job_id)
        if configuration is None or configuration.kind != "notification.send":
            return
        delivery_id = UUID(str(configuration.scope["delivery_id"]))
        session.rollback()
        with session.begin():
            mark_interrupted_sending_in_transaction(session, delivery_id=delivery_id, now=now)


def _validate_child_result(
    result: IsolatedProcessResult,
    acquired_lease: ExecutionLease,
) -> ChildJobResult:
    report = result.value
    if (
        not isinstance(report, ChildJobResult)
        or report.lease.job_id != acquired_lease.job_id
        or report.lease.worker_id != acquired_lease.worker_id
        or report.lease.epoch != acquired_lease.epoch
    ):
        raise JobProcessCrashedError("InvalidChildResult")
    return report


def _finalize_supervised_result(
    sessions: sessionmaker[Session],
    *,
    message: JobMessage,
    reference: MessageReference,
    report: ChildJobResult,
    lease_seconds: int,
    clock: Clock | None,
) -> None:
    finished_at = _now(clock)
    with sessions() as session:
        session.rollback()
        with session.begin():
            if message.kind == "notification.send":
                configuration = load_job_execution_configuration(session, job_id=message.job_id)
                if configuration is not None:
                    mark_interrupted_sending_in_transaction(
                        session,
                        delivery_id=UUID(str(configuration.scope["delivery_id"])),
                        now=finished_at,
                    )
            if message.kind == "webpage.collect":
                recover_webpage_collection_usage_in_transaction(
                    session,
                    owner_id=message.owner_id,
                    operation_id=message.operation_id,
                    finished_at=finished_at,
                )
            if message.kind == "source.hotlist" and message.source_key is not None:
                recover_hotlist_usage_in_transaction(
                    session,
                    owner_id=message.owner_id,
                    operation_id=message.operation_id,
                    source_key=message.source_key,
                    finished_at=finished_at,
                )
            execution = JobExecutionService(
                session,
                lease_seconds=lease_seconds,
                clock=lambda: finished_at,
            )
            if message.kind in {"keyword.search", "source.comments"}:
                recover_interrupted_collection_in_transaction(
                    session, execution=execution, lease=report.lease, finished_at=finished_at
                )
            if report.failure is not None:
                execution.record_failure_in_transaction(
                    report.lease,
                    message=reference,
                    failure=report.failure.to_failure(),
                    now=finished_at,
                )
            else:
                execution.complete_in_transaction(
                    report.lease,
                    message=reference,
                    completion=(
                        report.completion.to_completion() if report.completion is not None else None
                    ),
                    now=finished_at,
                )


def _worker_id() -> str:
    return f"{socket.gethostname()}:{os.getpid()}"


def _registered_job_handlers(
    sessions: sessionmaker[Session],
    settings: Settings,
    *,
    clock: Clock | None = None,
    media_storage: MediaObjectStorage | None = None,
) -> dict[str, JobHandler]:
    webpage_executor = WebPageCollectionExecutor(
        sessions,
        lease_seconds=settings.job_lease_seconds,
        timeout_seconds=settings.firecrawl_timeout_seconds,
        clock=clock,
        adapter_factory=lambda allowed_hosts: FirecrawlAdapter(
            base_url=settings.firecrawl_base_url,
            enabled=settings.firecrawl_enabled,
            allowed_hosts=allowed_hosts,
            max_response_bytes=settings.firecrawl_max_response_bytes,
            clock=clock,
        ),
    )
    keyword_executor = KeywordDiscoveryExecutor(
        sessions,
        lease_seconds=settings.job_lease_seconds,
        clock=clock,
    )
    comments_executor = CommentsExecutor(
        sessions,
        lease_seconds=settings.job_lease_seconds,
        clock=clock,
    )
    hotlist_executor = HotlistExecutor(
        sessions,
        lease_seconds=settings.job_lease_seconds,
        clock=clock,
    )
    analysis_executor = AnalysisAnnotateExecutor(sessions, settings, clock=clock)
    editorial_executor = EditorialExecutor(sessions, settings, clock=clock)
    selectbench_executor = SelectBenchExecutor(sessions, settings, clock=clock)
    translation_executor = ContentTranslationExecutor(sessions, settings, clock=clock)
    event_executor = EventClusterExecutor(sessions, settings, clock=clock)
    event_digest_executor = EventDigestExecutor(sessions, settings, clock=clock)
    event_consolidation_executor = EventConsolidationExecutor(sessions, settings, clock=clock)
    event_embedding_executor = EventEmbeddingExecutor(sessions, settings, clock=clock)
    event_signal_executor = EventSignalExecutor(sessions, settings, clock=clock)
    event_heat_executor = EventHeatExecutor(sessions, settings, clock=clock)
    edition_executor = EditionExecutor(sessions, settings, clock=clock)
    publication_executor = PublicationRepublishExecutor(
        sessions, indexing_enabled=settings.publication_indexing_enabled, clock=clock
    )
    daily_report_executor = DailyReportExecutor(sessions, settings=settings, clock=clock)
    private_export_executor = PrivateExportExecutor(
        sessions, media_storage, lease_seconds=settings.job_lease_seconds, clock=clock
    )
    knowledge_executor = KnowledgeExportExecutor(sessions, settings, clock=clock)
    notification_executor = NotificationExecutor(sessions, settings, clock=clock)
    notification_scan_executor = NotificationScanExecutor(sessions, settings, clock=clock)
    source_editorial_executor = EditorialSourceJobExecutor(
        sessions,
        settings,
        lease_seconds=settings.job_lease_seconds,
        clock=clock or (lambda: datetime.now(UTC)),
    )
    source_editorial_group_executor = EditorialXGroupJobExecutor(
        sessions,
        settings,
        lease_seconds=settings.job_lease_seconds,
        clock=clock or (lambda: datetime.now(UTC)),
    )
    source_editorial_preview_executor = EditorialSourcePreviewExecutor(
        sessions,
        settings,
        lease_seconds=settings.job_lease_seconds,
        clock=clock or (lambda: datetime.now(UTC)),
    )
    indexnow_executor = IndexNowExecutor(sessions, settings, clock=clock)
    source_icon_executor = SourceIconJobExecutor(
        sessions,
        settings,
        lease_seconds=settings.job_lease_seconds,
        clock=clock or (lambda: datetime.now(UTC)),
    )
    maintenance_executor = OperationsMaintenanceExecutor(sessions, settings, clock=clock)
    media_executor = (
        PublicationMediaExecutor(
            sessions,
            media_storage,
            allow_external_requests=settings.media_mirror_allow_external_requests,
            image_max_bytes=settings.media_mirror_image_max_bytes,
            video_max_bytes=settings.media_mirror_video_max_bytes,
            redirect_hosts=frozenset(settings.media_mirror_redirect_hosts),
            lease_seconds=settings.job_lease_seconds,
            clock=clock,
        )
        if media_storage is not None
        else None
    )
    codex_executor = CodexResetJobExecutor(
        sessions,
        settings,
        lease_seconds=settings.job_lease_seconds,
        clock=clock or (lambda: datetime.now(UTC)),
        notification_sink=lambda session, owner, monitor, intents: (
            enqueue_codex_notifications_in_transaction(
                session,
                owner_id=owner,
                monitor_id=monitor,
                intents=intents,
                now=_now(clock),
            )
            if settings.notifications_enabled
            else 0
        ),
    )
    leaderboard_executor = LeaderboardRefreshExecutor(
        sessions,
        allow_external_requests=settings.leaderboard_external_requests_enabled,
        artificial_analysis_api_key=(
            settings.leaderboard_artificial_analysis_api_key.get_secret_value()
            if settings.leaderboard_artificial_analysis_api_key
            else None
        ),
        github_token=(
            settings.leaderboard_github_token.get_secret_value()
            if settings.leaderboard_github_token
            else None
        ),
        max_requests_per_source=settings.leaderboard_max_requests_per_source,
        max_seconds_per_source=settings.leaderboard_max_seconds_per_source,
        solver_seconds=settings.leaderboard_solver_seconds,
        lease_seconds=settings.job_lease_seconds,
        clock=clock or (lambda: datetime.now(UTC)),
    )

    def collect_webpage(context: JobExecutionContext) -> JobCompletion:
        context.lease = webpage_executor.execute(context.message, context.lease)
        return JobCompletion(status=JobStatus.SUCCEEDED)

    def search_keyword(context: JobExecutionContext) -> JobCompletion:
        context.lease, completion = keyword_executor.execute(context.message, context.lease)
        return completion

    def collect_comments(context: JobExecutionContext) -> JobCompletion:
        context.lease, completion = comments_executor.execute(context.message, context.lease)
        return completion

    def collect_hotlist(context: JobExecutionContext) -> JobCompletion:
        context.lease, completion = hotlist_executor.execute(context.message, context.lease)
        return completion

    def annotate_content(context: JobExecutionContext) -> JobCompletion:
        result = analysis_executor.execute(context.message, context.lease)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"processed_items": result.processed_items},
            progress=JobProgress(
                stage=JobStage.ANALYSIS,
                items_saved=result.processed_items,
            ),
        )
        return result.completion

    def cluster_event(context: JobExecutionContext) -> JobCompletion:
        completion = event_executor.execute(context.message, context.lease)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"candidate_processed": True},
            progress=JobProgress(stage=JobStage.ANALYSIS, items_saved=1),
        )
        return completion

    def editorial_content(context: JobExecutionContext) -> JobCompletion:
        completion = editorial_executor.execute(context.message, context.lease)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"editorial_completed": True},
            progress=JobProgress(stage=JobStage.ANALYSIS, items_saved=1),
        )
        return completion

    def evaluate_selection(context: JobExecutionContext) -> JobCompletion:
        return selectbench_executor.execute(context.message, context.lease)

    def digest_event(context: JobExecutionContext) -> JobCompletion:
        completion = event_digest_executor.execute(context.message, context.lease)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"event_digest_completed": True},
            progress=JobProgress(stage=JobStage.ANALYSIS, items_saved=1),
        )
        return completion

    def consolidate_event(context: JobExecutionContext) -> JobCompletion:
        return event_consolidation_executor.execute(context.message, context.lease)

    def embed_event(context: JobExecutionContext) -> JobCompletion:
        return event_embedding_executor.execute(context.message, context.lease)

    def rematch_event_signals(context: JobExecutionContext) -> JobCompletion:
        return event_signal_executor.execute(context.message, context.lease)

    def refresh_source_icon(context: JobExecutionContext) -> JobCompletion | None:
        return source_icon_executor.execute(
            context.message, context.lease, cancelled=context.cancellation_requested
        )

    def translate_content(context: JobExecutionContext) -> JobCompletion:
        completion = translation_executor.execute(context.message, context.lease)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"translation_completed": True},
            progress=JobProgress(stage=JobStage.ANALYSIS, items_saved=1),
        )
        return completion

    def generate_daily_report(context: JobExecutionContext) -> JobCompletion:
        result = daily_report_executor.execute(context.message, context.lease)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {
                "report_id": str(result.report_id),
                "report_version": result.report_version,
            },
            progress=JobProgress(
                stage=JobStage.SAVE,
                items_saved=1,
            ),
        )
        return result.completion

    def heat_event(context: JobExecutionContext) -> JobCompletion:
        completion = event_heat_executor.execute(context.message)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"event_heat_completed": True},
            progress=JobProgress(stage=JobStage.SAVE, items_saved=1),
        )
        return completion

    def generate_edition(context: JobExecutionContext) -> JobCompletion:
        completion = edition_executor.execute(context.message, context.lease)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"edition_completed": True},
            progress=JobProgress(stage=JobStage.SAVE, items_saved=1),
        )
        return completion

    def republish(context: JobExecutionContext) -> JobCompletion | None:
        return publication_executor.execute(
            context.message,
            context.lease,
            cancelled=context.cancellation_requested,
            checkpoint=context.save_checkpoint,
        )

    def export_knowledge(context: JobExecutionContext) -> JobCompletion:
        completion = knowledge_executor.execute(context.message)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"exported": True},
            progress=JobProgress(stage=JobStage.SAVE, items_saved=1),
        )
        return completion

    def send_notification(context: JobExecutionContext) -> JobCompletion:
        return notification_executor.execute(context.message, context.lease)

    def scan_notifications(context: JobExecutionContext) -> JobCompletion:
        return notification_scan_executor.execute(context.message, context.lease)

    def poll_editorial_source(context: JobExecutionContext) -> JobCompletion | None:
        return source_editorial_executor.execute(
            context.message, context.lease, cancelled=context.cancellation_requested
        )

    def preview_editorial_source(context: JobExecutionContext) -> JobCompletion | None:
        return source_editorial_preview_executor.execute(
            context.message, context.lease, cancelled=context.cancellation_requested
        )

    def maintain(context: JobExecutionContext) -> JobCompletion:
        return maintenance_executor.execute(context.message, context.lease)

    def poll_editorial_group(context: JobExecutionContext) -> JobCompletion | None:
        return source_editorial_group_executor.execute(
            context.message, context.lease, cancelled=context.cancellation_requested
        )

    def submit_indexnow(context: JobExecutionContext) -> JobCompletion:
        return indexnow_executor.execute(context.message, context.lease)

    def mirror_media(context: JobExecutionContext) -> JobCompletion | None:
        if media_executor is None:
            raise JobExecutionFailure(
                error_code="media_storage_unavailable",
                category=JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                occurred_at=_now(clock),
                next_action="配置现有证据对象存储后人工重试",
                manual_retry_allowed=True,
            )
        return media_executor.execute(
            context.message, context.lease, cancelled=context.cancellation_requested
        )

    def scan_codex_resets(context: JobExecutionContext) -> JobCompletion | None:
        return codex_executor.execute(
            context.message, context.lease, cancelled=context.cancellation_requested
        )

    def refresh_leaderboard(context: JobExecutionContext) -> JobCompletion | None:
        return leaderboard_executor.execute(
            context.message, context.lease, cancelled=context.cancellation_requested
        )

    def export_private_file(context: JobExecutionContext) -> JobCompletion | None:
        return private_export_executor.execute(
            context.message, context.lease, cancelled=context.cancellation_requested
        )

    return {
        "report.export": export_private_file,
        "content.export": export_private_file,
        "analysis.annotate": annotate_content,
        "analysis.editorial": editorial_content,
        "analysis.selectbench": evaluate_selection,
        "analysis.translate": translate_content,
        "source.editorial.poll": poll_editorial_source,
        "source.editorial.ingest": poll_editorial_source,
        "source.editorial.x_group": poll_editorial_group,
        "source.editorial.preview": preview_editorial_source,
        "operations.maintenance": maintain,
        "monitor.codex_reset.tick": scan_codex_resets,
        "leaderboard.refresh": refresh_leaderboard,
        "events.cluster": cluster_event,
        "events.digest": digest_event,
        "events.consolidate": consolidate_event,
        "events.embed": embed_event,
        "events.signals": rematch_event_signals,
        "source.icons": refresh_source_icon,
        "events.heat": heat_event,
        "publication.republish": republish,
        "publication.indexnow": submit_indexnow,
        "publication.media_mirror": mirror_media,
        "report.edition": generate_edition,
        "keyword.search": search_keyword,
        "knowledge.export": export_knowledge,
        "notification.send": send_notification,
        "notification.scan": scan_notifications,
        "report.daily": generate_daily_report,
        "report.weekly": generate_daily_report,
        "source.comments": collect_comments,
        "source.hotlist": collect_hotlist,
        "webpage.collect": collect_webpage,
    }


def run_worker() -> None:
    settings = get_settings()
    configure_logging(settings.log_level)
    logger = structlog.get_logger("worker")
    stopping = Event()

    def request_stop(_signum: int, _frame: object) -> None:
        stopping.set()

    signal.signal(signal.SIGINT, request_stop)
    signal.signal(signal.SIGTERM, request_stop)

    engine = create_db_engine(settings)
    sessions = create_session_factory(engine, settings=settings)
    heartbeat = ProcessHeartbeatReporter(
        sessions, role="worker", enabled=settings.environment != "test"
    )
    media_storage = create_media_storage(settings)
    job_handlers = _registered_job_handlers(sessions, settings, media_storage=media_storage)
    producer = create_producer(settings)
    message_handler = create_job_message_handler(
        sessions,
        job_handlers,
        worker_id=_worker_id(),
        lease_seconds=settings.job_lease_seconds,
        supervisor=JobProcessSupervisor(
            startup_timeout_seconds=JOB_PROCESS_STARTUP_TIMEOUT_SECONDS,
            execution_timeout_seconds=settings.job_process_execution_timeout_seconds(
                "webpage.collect"
            ),
            terminate_grace_seconds=JOB_PROCESS_TERMINATE_GRACE_SECONDS,
        ),
        stopping=stopping,
        job_execution_timeout_seconds=settings.job_process_execution_timeout_seconds,
    )

    def publish_pending() -> None:
        with sessions() as session:
            OutboxService(session).publish_pending(
                lambda envelope: publish_outbox(
                    producer,
                    envelope,
                    timeout_seconds=settings.kafka_delivery_timeout_seconds,
                )
            )

    try:
        heartbeat.start()
        run_consumer_loop(
            settings,
            {JOB_ACCEPTED_TOPIC: message_handler},
            stopping,
            before_poll=publish_pending,
        )
    finally:
        heartbeat.stop()
        if media_storage is not None:
            media_storage.close()
        engine.dispose()
    logger.info("worker_stopped")
