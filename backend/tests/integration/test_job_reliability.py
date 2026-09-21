from __future__ import annotations

import os
import time
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from threading import Barrier
from uuid import UUID, uuid4

import pytest
from confluent_kafka import Consumer, Message
from confluent_kafka.admin import AdminClient, NewTopic
from redis import Redis
from redis.exceptions import RedisError
from sqlalchemy import Engine, create_engine, event, text
from sqlalchemy.orm import Session, sessionmaker

from core.config import Settings
from core.errors import ApplicationError
from jobs.execution import (
    JobExecutionService,
    MessageReference,
    StaleExecutionLeaseError,
    plan_catchup_windows,
)
from jobs.schemas import JobAcceptanceInput
from jobs.services import JobService, OutboxService
from worker.app import JobExecutionContext, create_job_message_handler
from worker.messaging import create_producer, decode_job_message, publish_outbox


@dataclass(frozen=True, slots=True)
class JobTestContext:
    engine: Engine
    sessions: sessionmaker[Session]
    owner_id: UUID


@pytest.fixture
def job_context() -> Iterator[JobTestContext]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    engine = create_engine(database_url, pool_size=4)
    sessions = sessionmaker(bind=engine, expire_on_commit=False)
    owner_id = uuid4()
    with engine.begin() as connection:
        connection.execute(
            text(
                "TRUNCATE processed_messages, job_attempts, outbox_messages, jobs, "
                "identity_sessions, identity_users"
            )
        )
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:id, 'job-test-owner', 'test-only-hash', 1, now(), now())"
            ),
            {"id": owner_id},
        )
    try:
        yield JobTestContext(engine=engine, sessions=sessions, owner_id=owner_id)
    finally:
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE processed_messages, job_attempts, outbox_messages, jobs, "
                    "identity_sessions, identity_users"
                )
            )
        engine.dispose()


def _command(*, operation_id: UUID | None = None, window: int = 7) -> JobAcceptanceInput:
    return JobAcceptanceInput(
        operation_id=operation_id or uuid4(),
        kind="monitor.collect",
        scope={"source_id": "account-1", "window": window},
    )


def _counts(context: JobTestContext) -> tuple[int, int]:
    with context.engine.connect() as connection:
        jobs = connection.execute(text("SELECT count(*) FROM jobs")).scalar_one()
        outbox = connection.execute(text("SELECT count(*) FROM outbox_messages")).scalar_one()
    return int(jobs), int(outbox)


def _poll_message(consumer: Consumer, timeout_seconds: float = 10) -> Message:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        message = consumer.poll(0.2)
        if message is None:
            continue
        if message.error() is not None:
            raise RuntimeError(str(message.error()))
        return message
    raise AssertionError("Kafka message was not received before the timeout")


def _wait_for_assignment(consumer: Consumer, timeout_seconds: float = 10) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        consumer.poll(0.1)
        if consumer.assignment():
            return
    raise AssertionError("Kafka consumer did not receive a partition assignment")


def test_lost_response_retry_returns_the_original_job_and_one_outbox(
    job_context: JobTestContext,
) -> None:
    command = _command()
    with job_context.sessions() as session:
        first = JobService(session).accept(owner_id=job_context.owner_id, command=command)
    with job_context.sessions() as session:
        repeated = JobService(session).accept(owner_id=job_context.owner_id, command=command)

    assert repeated == first
    assert first.status == "queued"
    assert _counts(job_context) == (1, 1)
    with job_context.engine.connect() as connection:
        payload = connection.execute(text("SELECT payload FROM outbox_messages")).scalar_one()
    assert payload == {
        "job_id": str(first.id),
        "kind": command.kind,
        "operation_id": str(command.operation_id),
        "owner_id": str(job_context.owner_id),
    }


def test_reused_operation_id_with_different_scope_is_rejected(
    job_context: JobTestContext,
) -> None:
    operation_id = uuid4()
    with job_context.sessions() as session:
        original = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(operation_id=operation_id, window=7),
        )
    with (
        job_context.sessions() as session,
        pytest.raises(ApplicationError, match="idempotency_conflict") as captured,
    ):
        JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(operation_id=operation_id, window=8),
        )

    assert captured.value.code == "idempotency_conflict"
    assert _counts(job_context) == (1, 1)
    with job_context.engine.connect() as connection:
        stored_scope = connection.execute(
            text("SELECT scope FROM jobs WHERE id = :id"), {"id": original.id}
        ).scalar_one()
    assert stored_scope["window"] == 7


def test_transaction_failure_creates_neither_job_nor_outbox(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:

        def fail_after_flush(_: Session, __: object) -> None:
            raise RuntimeError("injected failure after transaction flush")

        event.listen(session, "after_flush_postexec", fail_after_flush)
        try:
            with pytest.raises(RuntimeError, match="injected failure"):
                JobService(session).accept(
                    owner_id=job_context.owner_id,
                    command=_command(),
                )
        finally:
            event.remove(session, "after_flush_postexec", fail_after_flush)

    assert _counts(job_context) == (0, 0)


def test_concurrent_retries_return_one_job_and_one_outbox(
    job_context: JobTestContext,
) -> None:
    command = _command()
    barrier = Barrier(2)

    def accept() -> UUID:
        with job_context.sessions() as session:
            barrier.wait(timeout=5)
            return (
                JobService(session)
                .accept(
                    owner_id=job_context.owner_id,
                    command=command,
                )
                .id
            )

    with ThreadPoolExecutor(max_workers=2) as executor:
        first = executor.submit(accept)
        second = executor.submit(accept)

    assert first.result() == second.result()
    assert _counts(job_context) == (1, 1)


def test_expired_lease_recovers_checkpoint_and_fences_old_worker(
    job_context: JobTestContext,
) -> None:
    with job_context.sessions() as session:
        job = JobService(session).accept(
            owner_id=job_context.owner_id,
            command=_command(),
        )
    clock = [job.created_at + timedelta(seconds=1)]
    with job_context.sessions() as session:
        service = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0])
        old = service.acquire(job_id=job.id, worker_id="worker-old")
        old = service.save_checkpoint(old, sequence=1, checkpoint={"page": 1})

    clock[0] += timedelta(seconds=61)
    with job_context.sessions() as session:
        service = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0])
        recovered = service.acquire(job_id=job.id, worker_id="worker-new")

    assert recovered.epoch == old.epoch + 1
    assert recovered.checkpoint_sequence == 1
    assert recovered.checkpoint == {"page": 1}

    with job_context.sessions() as session:
        service = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0])
        with pytest.raises(StaleExecutionLeaseError):
            service.save_checkpoint(old, sequence=2, checkpoint={"page": 2})

    reference = MessageReference(
        message_id=uuid4(),
        topic="hotkey.tests.jobs",
        partition=0,
        offset=0,
    )
    with job_context.sessions() as session:
        service = JobExecutionService(session, lease_seconds=60, clock=lambda: clock[0])
        recovered = service.save_checkpoint(
            recovered,
            sequence=2,
            checkpoint={"page": 2},
        )
        service.complete(recovered, message=reference)

    with job_context.engine.connect() as connection:
        job_row = connection.execute(
            text(
                "SELECT status, checkpoint_sequence, checkpoint, lease_owner "
                "FROM jobs WHERE id = :id"
            ),
            {"id": job.id},
        ).one()
        attempts = connection.execute(
            text(
                "SELECT lease_epoch, outcome FROM job_attempts "
                "WHERE job_id = :id ORDER BY lease_epoch"
            ),
            {"id": job.id},
        ).all()

    assert tuple(job_row) == ("succeeded", 2, {"page": 2}, None)
    assert attempts == [(1, "expired"), (2, "succeeded")]


def test_concurrent_schedule_window_acceptance_creates_one_job(
    job_context: JobTestContext,
) -> None:
    start = datetime(2026, 9, 21, 0, tzinfo=UTC)
    window = plan_catchup_windows(
        due_from=start,
        due_until=start + timedelta(hours=1),
        cadence=timedelta(hours=1),
        max_windows=3,
    ).windows[0]
    barrier = Barrier(2)

    def accept() -> UUID:
        with job_context.sessions() as session:
            barrier.wait(timeout=5)
            return (
                JobService(session)
                .accept_schedule_window(
                    owner_id=job_context.owner_id,
                    kind="monitor.collect",
                    schedule_key="topic-1",
                    window=window,
                    scope={"source_id": "account-1"},
                )
                .id
            )

    with ThreadPoolExecutor(max_workers=2) as executor:
        first = executor.submit(accept)
        second = executor.submit(accept)

    assert first.result() == second.result()
    assert _counts(job_context) == (1, 1)


def test_real_kafka_redelivery_rebalance_and_redis_loss_recover_once(
    job_context: JobTestContext,
) -> None:
    bootstrap_servers = os.getenv("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS")
    if bootstrap_servers is None:
        pytest.skip("HOTKEY_TEST_KAFKA_BOOTSTRAP_SERVERS is required for Kafka integration")

    topic = f"hotkey.tests.jobs.{uuid4().hex}"
    group_id = f"hotkey-tests-{uuid4().hex}"
    admin = AdminClient({"bootstrap.servers": bootstrap_servers})
    admin.create_topics([NewTopic(topic, num_partitions=1, replication_factor=1)])[topic].result(10)

    consumer_config = {
        "bootstrap.servers": bootstrap_servers,
        "group.id": group_id,
        "enable.auto.commit": False,
        "enable.auto.offset.store": False,
        "auto.offset.reset": "earliest",
        "session.timeout.ms": 6_000,
        "max.poll.interval.ms": 120_000,
    }
    first_consumer: Consumer | None = Consumer(consumer_config)
    second_consumer: Consumer | None = None
    try:
        first_consumer.subscribe([topic])
        _wait_for_assignment(first_consumer)

        with job_context.sessions() as session:
            job = JobService(session).accept(
                owner_id=job_context.owner_id,
                command=_command(),
            )
        with job_context.engine.begin() as connection:
            connection.execute(
                text("UPDATE outbox_messages SET topic = :topic WHERE aggregate_id = :job_id"),
                {"topic": topic, "job_id": job.id},
            )

        settings = Settings(
            database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
            kafka_bootstrap_servers=bootstrap_servers,
            kafka_delivery_timeout_seconds=10,
        )
        producer = create_producer(settings)

        def publish_then_lose_database_ack(envelope) -> None:
            publish_outbox(producer, envelope, timeout_seconds=10)
            raise RuntimeError("injected failure after Kafka delivery")

        with (
            job_context.sessions() as session,
            pytest.raises(RuntimeError, match="after Kafka delivery"),
        ):
            OutboxService(session).publish_pending(publish_then_lose_database_ack)

        first_message = _poll_message(first_consumer)
        first_body, first_reference = decode_job_message(first_message)
        assert first_body.job_id == job.id

        clock = [job.created_at + timedelta(seconds=1)]
        interrupted_leases = []

        def interrupt(context: JobExecutionContext) -> None:
            context.save_checkpoint(1, {"page": 1})
            interrupted_leases.append(context.lease)
            raise RuntimeError("injected worker interruption")

        interrupted_handler = create_job_message_handler(
            job_context.sessions,
            {"monitor.collect": interrupt},
            worker_id="worker-before-rebalance",
            lease_seconds=60,
            clock=lambda: clock[0],
        )
        with pytest.raises(RuntimeError, match="worker interruption"):
            interrupted_handler(first_message)
        old = interrupted_leases[0]

        first_consumer.close()
        first_consumer = None
        with job_context.sessions() as session:
            assert (
                OutboxService(session).publish_pending(
                    lambda envelope: publish_outbox(producer, envelope, timeout_seconds=10)
                )
                == 1
            )

        unreachable_redis = Redis.from_url(
            "redis://127.0.0.1:1/0",
            socket_connect_timeout=0.1,
            socket_timeout=0.1,
        )
        with pytest.raises(RedisError):
            unreachable_redis.ping()

        clock[0] += timedelta(seconds=61)
        second_consumer = Consumer(consumer_config)
        second_consumer.subscribe([topic])
        replayed_message = _poll_message(second_consumer)
        replayed_body, replayed_reference = decode_job_message(replayed_message)
        assert replayed_body.message_id == first_body.message_id
        assert replayed_reference.offset == first_reference.offset

        recovered_calls = []

        def recover(context: JobExecutionContext) -> None:
            recovered_calls.append(context.lease.epoch)
            assert context.lease.checkpoint == {"page": 1}
            with (
                pytest.raises(StaleExecutionLeaseError),
                job_context.sessions() as session,
            ):
                JobExecutionService(
                    session,
                    lease_seconds=60,
                    clock=lambda: clock[0],
                ).save_checkpoint(old, sequence=2, checkpoint={"page": 2})
            context.save_checkpoint(2, {"page": 2})

        recovered_handler = create_job_message_handler(
            job_context.sessions,
            {"monitor.collect": recover},
            worker_id="worker-after-rebalance",
            lease_seconds=60,
            clock=lambda: clock[0],
        )
        recovered_handler(replayed_message)
        assert recovered_calls == [old.epoch + 1]
        second_consumer.commit(message=replayed_message, asynchronous=False)

        duplicate_message = _poll_message(second_consumer)
        duplicate_body, _duplicate_reference = decode_job_message(duplicate_message)
        assert duplicate_body.message_id == replayed_body.message_id
        recovered_handler(duplicate_message)
        assert recovered_calls == [old.epoch + 1]
        second_consumer.commit(message=duplicate_message, asynchronous=False)

        with job_context.engine.connect() as connection:
            row = connection.execute(
                text(
                    "SELECT j.status, j.checkpoint_sequence, "
                    "(SELECT count(*) FROM processed_messages WHERE job_id = j.id), "
                    "(SELECT count(*) FROM job_attempts WHERE job_id = j.id) "
                    "FROM jobs j WHERE j.id = :job_id"
                ),
                {"job_id": job.id},
            ).one()
            published_at = connection.execute(
                text("SELECT published_at FROM outbox_messages WHERE aggregate_id = :job_id"),
                {"job_id": job.id},
            ).scalar_one()

        assert tuple(row) == ("succeeded", 2, 1, 2)
        assert published_at is not None
    finally:
        if first_consumer is not None:
            first_consumer.close()
        if second_consumer is not None:
            second_consumer.close()
        admin.delete_topics([topic])[topic].result(10)
