from __future__ import annotations

import os
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from threading import Barrier
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, event, text
from sqlalchemy.orm import Session, sessionmaker

from core.errors import ApplicationError
from jobs.schemas import JobAcceptanceInput
from jobs.services import JobService


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
            text("TRUNCATE outbox_messages, jobs, identity_sessions, identity_users")
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
                text("TRUNCATE outbox_messages, jobs, identity_sessions, identity_users")
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
