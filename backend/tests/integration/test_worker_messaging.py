from __future__ import annotations

import os
from contextlib import suppress
from datetime import UTC, datetime, timedelta
from threading import Event, Timer
from uuid import uuid4

import pytest
from confluent_kafka import Message, Producer
from confluent_kafka.admin import AdminClient, NewTopic

from core.config import Settings
from worker.messaging import MessageDeferredError, run_consumer_loop


def test_deferred_message_is_redelivered_after_consumer_recreation() -> None:
    bootstrap_servers = os.getenv("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS")
    if bootstrap_servers is None:
        pytest.skip("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS is required for Kafka integration")

    suffix = uuid4().hex
    topic = f"hotkey.tests.worker.defer.{suffix}"
    group_id = f"hotkey-tests-worker-defer-{suffix}"
    admin = AdminClient({"bootstrap.servers": bootstrap_servers})
    admin.create_topics([NewTopic(topic, num_partitions=1, replication_factor=1)])[topic].result(10)
    producer = Producer({"bootstrap.servers": bootstrap_servers})
    stopping = Event()
    calls: list[tuple[int, int]] = []
    timeout = Timer(10, stopping.set)
    settings = Settings(
        database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
        kafka_bootstrap_servers=bootstrap_servers,
        kafka_group_id=group_id,
    )

    try:
        producer.produce(topic, key=b"job", value=b"payload")
        assert producer.flush(10) == 0

        def handle(message: Message) -> None:
            calls.append((message.partition(), message.offset()))
            if len(calls) == 1:
                raise MessageDeferredError(datetime.now(UTC) + timedelta(milliseconds=100))
            stopping.set()

        timeout.start()
        run_consumer_loop(settings, {topic: handle}, stopping)
    finally:
        timeout.cancel()
        with suppress(Exception):
            admin.delete_topics([topic], operation_timeout=10)[topic].result(10)

    assert calls == [(0, 0), (0, 0)]
