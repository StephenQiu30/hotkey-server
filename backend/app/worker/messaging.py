from __future__ import annotations

import json
from collections.abc import Callable, Mapping
from threading import Event
from uuid import UUID

import structlog
from confluent_kafka import Consumer, KafkaError, Message, Producer
from pydantic import TypeAdapter

from core.config import Settings
from jobs.execution import MessageReference
from jobs.schemas import JobMessage
from jobs.services import OutboxEnvelope

MessageHandler = Callable[[Message], None]
BeforePoll = Callable[[], None]


class MessagePublishError(RuntimeError):
    """Kafka did not durably acknowledge an outbox message."""


def create_consumer(settings: Settings) -> Consumer:
    return Consumer(
        {
            "bootstrap.servers": settings.kafka_bootstrap_servers,
            "group.id": settings.kafka_group_id,
            "enable.auto.commit": False,
            "enable.auto.offset.store": False,
            "auto.offset.reset": "earliest",
            "max.poll.interval.ms": settings.kafka_max_poll_interval_seconds * 1000,
        }
    )


def create_producer(settings: Settings) -> Producer:
    return Producer(
        {
            "bootstrap.servers": settings.kafka_bootstrap_servers,
            "enable.idempotence": True,
            "acks": "all",
            "request.timeout.ms": 5_000,
            "delivery.timeout.ms": settings.kafka_delivery_timeout_seconds * 1000,
        }
    )


def publish_outbox(
    producer: Producer,
    envelope: OutboxEnvelope,
    *,
    timeout_seconds: float,
) -> None:
    delivery_errors: list[KafkaError | None] = []

    def delivered(error: KafkaError | None, _message: Message) -> None:
        delivery_errors.append(error)

    producer.produce(
        envelope.topic,
        key=str(envelope.message_key).encode(),
        value=json.dumps(
            envelope.message_body(),
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode(),
        headers={"hotkey-message-id": str(envelope.message_id)},
        on_delivery=delivered,
    )
    remaining = producer.flush(timeout_seconds)
    if remaining != 0 or len(delivery_errors) != 1 or delivery_errors[0] is not None:
        raise MessagePublishError("Kafka did not acknowledge the outbox message")


def decode_job_message(message: Message) -> tuple[JobMessage, MessageReference]:
    topic = message.topic()
    value = message.value()
    key = message.key()
    partition = message.partition()
    offset = message.offset()
    if topic is None or value is None or key is None or partition is None or offset is None:
        raise ValueError("job message is missing topic, key, payload, or position")
    body: JobMessage = TypeAdapter(JobMessage).validate_json(value)
    try:
        key_id = UUID(key.decode())
    except (UnicodeDecodeError, ValueError) as error:
        raise ValueError("job message key must be a UUID") from error
    if key_id != body.job_id:
        raise ValueError("job message key does not match its job ID")
    headers = message.headers()
    header_items = headers.items() if isinstance(headers, dict) else headers or []
    message_ids = [
        header_value.encode() if isinstance(header_value, str) else header_value
        for header_name, header_value in header_items
        if header_name == "hotkey-message-id"
    ]
    if message_ids != [str(body.message_id).encode()]:
        raise ValueError("job message ID header is missing or inconsistent")
    return body, MessageReference(
        message_id=body.message_id,
        topic=topic,
        partition=partition,
        offset=offset,
    )


def process_message(
    consumer: Consumer,
    message: Message,
    handlers: Mapping[str, MessageHandler],
) -> None:
    topic = message.topic()
    if topic is None or topic not in handlers:
        raise RuntimeError(f"message handler is not registered for topic: {topic}")
    handlers[topic](message)
    committed = consumer.commit(message=message, asynchronous=False)
    if committed is not None and any(partition.error is not None for partition in committed):
        raise RuntimeError("Kafka offset commit returned a partition error")


def run_consumer_loop(
    settings: Settings,
    handlers: Mapping[str, MessageHandler],
    stopping: Event,
    *,
    before_poll: BeforePoll | None = None,
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
            if before_poll is not None:
                before_poll()
            message = consumer.poll(timeout=1.0)
            if message is None:
                continue
            error = message.error()
            if error is not None:
                logger.error("kafka_consume_failed", error_code=error.code())
                continue

            try:
                process_message(consumer, message, handlers)
            except Exception as error:
                logger.exception(
                    "message_processing_failed",
                    topic=message.topic(),
                    partition=message.partition(),
                    offset=message.offset(),
                    exception_type=type(error).__name__,
                )
                raise
    finally:
        consumer.close()
