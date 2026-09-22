from __future__ import annotations

import os
import signal
import socket
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from threading import Event

import structlog
from confluent_kafka import Message
from sqlalchemy.orm import Session, sessionmaker
from structlog.contextvars import bound_contextvars

from content.collection import WebPageCollectionExecutor
from core.config import Settings, get_settings
from core.logging import configure_logging
from db.session import create_db_engine, create_session_factory
from jobs.execution import (
    CheckpointValue,
    Clock,
    ExecutionLease,
    JobCompletion,
    JobExecutionFailure,
    JobExecutionService,
    JobProgress,
)
from jobs.schemas import JobFailureCategory, JobMessage, JobStatus
from jobs.services import JOB_ACCEPTED_TOPIC, OutboxService
from sources.adapters.firecrawl import FirecrawlAdapter
from worker.messaging import (
    MessageHandler,
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


def create_job_message_handler(
    sessions: sessionmaker[Session],
    handlers: Mapping[str, JobHandler],
    *,
    worker_id: str,
    lease_seconds: int,
    clock: Clock | None = None,
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
                lease = execution.acquire(job_id=body.job_id, worker_id=worker_id)
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


def _worker_id() -> str:
    return f"{socket.gethostname()}:{os.getpid()}"


def _registered_job_handlers(
    sessions: sessionmaker[Session],
    settings: Settings,
    *,
    clock: Clock | None = None,
) -> dict[str, JobHandler]:
    executor = WebPageCollectionExecutor(
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

    def collect_webpage(context: JobExecutionContext) -> JobCompletion:
        context.lease = executor.execute(context.message, context.lease)
        return JobCompletion(status=JobStatus.SUCCEEDED)

    return {"webpage.collect": collect_webpage}


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
