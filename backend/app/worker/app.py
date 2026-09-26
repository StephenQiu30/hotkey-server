from __future__ import annotations

import os
import signal
import socket
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

from analysis.services import AnalysisAnnotateExecutor
from content.collection import (
    WebPageCollectionExecutor,
    recover_webpage_collection_usage_in_transaction,
)
from content.comments_execution import CommentsExecutor
from content.discovery_execution import KeywordDiscoveryExecutor
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
from notifications.executor import NotificationExecutor
from notifications.services import mark_interrupted_sending_in_transaction
from reports.services import DailyReportExecutor
from sources.adapters.firecrawl import FirecrawlAdapter
from worker.execution import (
    IsolatedProcessResult,
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
    sessions = create_session_factory(engine)
    try:
        handler = _registered_job_handlers(sessions, settings).get(message.kind)
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
    job_execution_timeout_seconds: Callable[[str], float] | None = None,
) -> MessageHandler:
    def handle(message: Message) -> None:
        body, reference = decode_job_message(message)
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
                try:
                    result = supervisor.run(
                        _run_job_in_child,
                        (body, lease, lease_seconds),
                        cancellation_requested=lambda: _cancellation_requested(
                            sessions,
                            lease=lease,
                            lease_seconds=lease_seconds,
                            clock=clock,
                        ),
                        stopping=stopping,
                        execution_timeout_seconds=(
                            job_execution_timeout_seconds(body.kind)
                            if job_execution_timeout_seconds is not None
                            else None
                        ),
                    )
                except JobProcessShutdownError as error:
                    if body.kind == "notification.send":
                        _mark_interrupted_notification(sessions, message=body, now=_now(clock))
                    raise StopProcessingMessageError from error
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
            execution = JobExecutionService(
                session,
                lease_seconds=lease_seconds,
                clock=lambda: finished_at,
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
    analysis_executor = AnalysisAnnotateExecutor(sessions, settings, clock=clock)
    daily_report_executor = DailyReportExecutor(sessions, clock=clock)
    knowledge_executor = KnowledgeExportExecutor(sessions, settings, clock=clock)
    notification_executor = NotificationExecutor(sessions, settings, clock=clock)

    def collect_webpage(context: JobExecutionContext) -> JobCompletion:
        context.lease = webpage_executor.execute(context.message, context.lease)
        return JobCompletion(status=JobStatus.SUCCEEDED)

    def search_keyword(context: JobExecutionContext) -> JobCompletion:
        context.lease, completion = keyword_executor.execute(context.message, context.lease)
        return completion

    def collect_comments(context: JobExecutionContext) -> JobCompletion:
        context.lease, completion = comments_executor.execute(context.message, context.lease)
        return completion

    def annotate_content(context: JobExecutionContext) -> JobCompletion:
        result = analysis_executor.execute(context.message)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"processed_items": result.processed_items},
            progress=JobProgress(
                stage=JobStage.ANALYSIS,
                items_saved=result.processed_items,
            ),
        )
        return result.completion

    def generate_daily_report(context: JobExecutionContext) -> JobCompletion:
        result = daily_report_executor.execute(context.message)
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

    def export_knowledge(context: JobExecutionContext) -> JobCompletion:
        completion = knowledge_executor.execute(context.message)
        context.save_checkpoint(
            context.lease.checkpoint_sequence + 1,
            {"exported": True},
            progress=JobProgress(stage=JobStage.SAVE, items_saved=1),
        )
        return completion

    def send_notification(context: JobExecutionContext) -> JobCompletion:
        return notification_executor.execute(context.message)

    return {
        "analysis.annotate": annotate_content,
        "keyword.search": search_keyword,
        "knowledge.export": export_knowledge,
        "notification.send": send_notification,
        "report.daily": generate_daily_report,
        "source.comments": collect_comments,
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
    sessions = create_session_factory(engine)
    job_handlers = _registered_job_handlers(sessions, settings)
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
        run_consumer_loop(
            settings,
            {JOB_ACCEPTED_TOPIC: message_handler},
            stopping,
            before_poll=publish_pending,
        )
    finally:
        engine.dispose()
    logger.info("worker_stopped")
