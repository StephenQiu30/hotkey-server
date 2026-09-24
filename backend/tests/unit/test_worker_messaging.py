from __future__ import annotations

from datetime import UTC, datetime, timedelta
from threading import Event
from typing import cast

import pytest
from confluent_kafka import Consumer, Message

import worker.messaging as messaging
from core.config import Settings
from worker.messaging import MessageDeferredError, process_message, run_consumer_loop


class FakeMessage:
    def topic(self) -> str:
        return "hotkey.jobs.accepted.v2"

    def error(self) -> None:
        return None

    def partition(self) -> int:
        return 0

    def offset(self) -> int:
        return 0


class FakeConsumer:
    def __init__(self, message: FakeMessage | None = None) -> None:
        self.commits = 0
        self.message = message
        self.closed = False
        self.subscriptions: list[list[str]] = []

    def subscribe(self, topics: list[str]) -> None:
        self.subscriptions.append(topics)

    def poll(self, timeout: float) -> FakeMessage | None:
        del timeout
        return self.message

    def commit(self, **_: object) -> list[object]:
        self.commits += 1
        return []

    def close(self) -> None:
        self.closed = True


def test_message_failure_does_not_commit_a_later_position() -> None:
    consumer = FakeConsumer()
    message = FakeMessage()

    def fail(_: Message) -> None:
        raise RuntimeError("injected handler failure")

    with pytest.raises(RuntimeError, match="injected handler failure"):
        process_message(
            cast(Consumer, consumer),
            cast(Message, message),
            {"hotkey.jobs.accepted.v2": fail},
        )

    assert consumer.commits == 0


def test_message_success_commits_exactly_that_message() -> None:
    consumer = FakeConsumer()
    message = FakeMessage()
    handled: list[Message] = []

    process_message(
        cast(Consumer, consumer),
        cast(Message, message),
        {"hotkey.jobs.accepted.v2": handled.append},
    )

    assert handled == [message]
    assert consumer.commits == 1


def test_deferred_message_recreates_consumer_without_committing_before_retry(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    message = FakeMessage()
    first_consumer = FakeConsumer(message)
    second_consumer = FakeConsumer(message)
    consumers = iter((first_consumer, second_consumer))
    monkeypatch.setattr(
        messaging,
        "create_consumer",
        lambda _settings: cast(Consumer, next(consumers)),
    )
    stopping = Event()
    calls = 0

    def handle(_message: Message) -> None:
        nonlocal calls
        calls += 1
        if calls == 1:
            raise MessageDeferredError(datetime.now(UTC) + timedelta(milliseconds=30))
        stopping.set()

    started_at = datetime.now(UTC)
    run_consumer_loop(
        cast(Settings, object()),
        {"hotkey.jobs.accepted.v2": handle},
        stopping,
    )

    assert calls == 2
    assert first_consumer.closed
    assert first_consumer.commits == 0
    assert second_consumer.closed
    assert second_consumer.commits == 1
    assert datetime.now(UTC) - started_at >= timedelta(milliseconds=20)
