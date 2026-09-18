from __future__ import annotations

from collections.abc import Callable, Mapping
from threading import Event

import structlog
from confluent_kafka import Consumer, Message

from core.config import Settings

MessageHandler = Callable[[Message], None]


def create_consumer(settings: Settings) -> Consumer:
    return Consumer(
        {
            "bootstrap.servers": settings.kafka_bootstrap_servers,
            "group.id": settings.kafka_group_id,
            "enable.auto.commit": False,
            "auto.offset.reset": "earliest",
        }
    )


def run_consumer_loop(
    settings: Settings,
    handlers: Mapping[str, MessageHandler],
    stopping: Event,
) -> None:
    logger = structlog.get_logger("worker")
    topics = sorted(handlers)
    if not topics:
        logger.info("worker_idle", reason="no_message_handlers_registered")
        return

    consumer = create_consumer(settings)
    consumer.subscribe(topics)
    logger.info("worker_started", topics=topics)
    try:
        while not stopping.is_set():
            message = consumer.poll(timeout=1.0)
            if message is None:
                continue
            error = message.error()
            if error is not None:
                logger.error("kafka_consume_failed", error_code=error.code())
                continue

            topic = message.topic()
            if topic is None or topic not in handlers:
                logger.error("message_handler_missing", topic=topic)
                continue

            try:
                handlers[topic](message)
            except Exception as error:
                logger.exception(
                    "message_processing_failed",
                    topic=topic,
                    partition=message.partition(),
                    offset=message.offset(),
                    exception_type=type(error).__name__,
                )
                continue
            consumer.commit(message=message, asynchronous=False)
    finally:
        consumer.close()
