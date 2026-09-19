from __future__ import annotations

import signal
from threading import Event

import structlog

from core.config import get_settings
from core.logging import configure_logging
from worker.messaging import MessageHandler, run_consumer_loop


def run_worker() -> None:
    settings = get_settings()
    configure_logging(settings.log_level)
    logger = structlog.get_logger("worker")
    stopping = Event()

    def request_stop(_signum: int, _frame: object) -> None:
        stopping.set()

    signal.signal(signal.SIGINT, request_stop)
    signal.signal(signal.SIGTERM, request_stop)

    handlers: dict[str, MessageHandler] = {}
    run_consumer_loop(settings, handlers, stopping)
    logger.info("worker_stopped")
