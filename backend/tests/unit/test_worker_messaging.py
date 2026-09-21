from __future__ import annotations

from typing import cast

import pytest
from confluent_kafka import Consumer, Message

from worker.messaging import process_message


class FakeMessage:
    def topic(self) -> str:
        return "hotkey.jobs.accepted.v2"


class FakeConsumer:
    def __init__(self) -> None:
        self.commits = 0

    def commit(self, **_: object) -> list[object]:
        self.commits += 1
        return []


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
