from __future__ import annotations

import os
import signal
import socket
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from threading import Event

import structlog
from confluent_kafka import Message
from sqlalchemy.orm import Session, sessionmaker

from core.config import get_settings
from core.logging import configure_logging
from db.session import create_db_engine, create_session_factory
from jobs.execution import CheckpointValue, Clock, ExecutionLease, JobExecutionService
from jobs.schemas import JobAcceptedMessage
from jobs.services import JOB_ACCEPTED_TOPIC, OutboxService
from worker.messaging import (
    MessageHandler,
    create_producer,
    decode_job_message,
    publish_outbox,
    run_consumer_loop,
)

JobHandler = Callable[["JobExecutionContext"], None]


@dataclass(slots=True)
class JobExecutionContext:
    message: JobAcceptedMessage
    lease: ExecutionLease
    _sessions: sessionmaker[Session]
    _lease_seconds: int
    _clock: Clock | None

    def save_checkpoint(
        self,
        sequence: int,
        checkpoint: Mapping[str, CheckpointValue],
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
            )


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
        handler = handlers.get(body.kind)
        if handler is None:
            raise RuntimeError(f"job handler is not registered for kind: {body.kind}")

        with sessions() as session:
            execution = JobExecutionService(
                session,
                lease_seconds=lease_seconds,
                clock=clock,
            )
            if execution.is_processed(body.message_id):
                return
            lease = execution.acquire(job_id=body.job_id, worker_id=worker_id)

        context = JobExecutionContext(
            message=body,
            lease=lease,
            _sessions=sessions,
            _lease_seconds=lease_seconds,
            _clock=clock,
        )
        handler(context)
        with sessions() as session:
            JobExecutionService(
                session,
                lease_seconds=lease_seconds,
                clock=clock,
            ).complete(context.lease, message=reference)

    return handle


def _worker_id() -> str:
    return f"{socket.gethostname()}:{os.getpid()}"


def _registered_job_handlers() -> dict[str, JobHandler]:
    return {}


def run_worker() -> None:
    settings = get_settings()
    configure_logging(settings.log_level)
    logger = structlog.get_logger("worker")
    stopping = Event()

    def request_stop(_signum: int, _frame: object) -> None:
        stopping.set()

    signal.signal(signal.SIGINT, request_stop)
    signal.signal(signal.SIGTERM, request_stop)

    job_handlers = _registered_job_handlers()
    if not job_handlers:
        run_consumer_loop(settings, {}, stopping)
        logger.info("worker_stopped")
        return

    engine = create_db_engine(settings)
    sessions = create_session_factory(engine)
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
