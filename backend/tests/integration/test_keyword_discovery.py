from __future__ import annotations

import os
from datetime import UTC, datetime
from uuid import uuid4

import pytest
from sqlalchemy import create_engine, select, text
from sqlalchemy.orm import Session

from content.discovery import plan_keyword_discovery
from content.schemas import KeywordDiscoveryRunInput
from core.errors import ApplicationError
from jobs.models import Job, OutboxMessage
from jobs.services import JobService


def test_query_plan_persists_independent_jobs_without_leaking_query_to_outbox() -> None:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    engine = create_engine(database_url)
    owner_id = uuid4()
    run = KeywordDiscoveryRunInput(
        run_id=uuid4(),
        configuration_ref="topic:fixture",
        configuration_version=3,
        source_key="x",
        primary_query="product fault",
        starts_at=datetime(2026, 9, 22, tzinfo=UTC),
        ends_at=datetime(2026, 9, 23, tzinfo=UTC),
        page_size=20,
        latest_max_pages=3,
        latest_max_requests=6,
        top_max_pages=2,
        top_max_requests=4,
    )
    commands = plan_keyword_discovery(run)
    try:
        with Session(engine) as session, session.begin():
            session.execute(
                text(
                    "INSERT INTO identity_users "
                    "(id, username, password_hash, credential_version, created_at, updated_at) "
                    "VALUES (:id, :username, 'test-only-hash', 1, now(), now())"
                ),
                {"id": owner_id, "username": f"discovery-{owner_id}"},
            )
            accepted = [
                JobService(session).accept_in_transaction(owner_id=owner_id, command=command)
                for command in commands
            ]

        with Session(engine) as session:
            jobs = session.scalars(select(Job).where(Job.owner_id == owner_id)).all()
            messages = session.scalars(
                select(OutboxMessage).where(
                    OutboxMessage.aggregate_id.in_([job.id for job in jobs])
                )
            ).all()
            assert len(jobs) == len(messages) == len(accepted) == 2
            assert {job.scope["sort_key"] for job in jobs} == {"latest", "top"}
            assert {job.scope["query"] for job in jobs} == {"product fault"}
            assert all("query" not in message.payload for message in messages)
            assert all("product fault" not in str(message.payload) for message in messages)

        for command, original in zip(commands, accepted, strict=True):
            with Session(engine) as session:
                repeated = JobService(session).accept(owner_id=owner_id, command=command)
            assert repeated.id == original.id

        changed = run.model_copy(update={"primary_query": "different search"})
        with Session(engine) as session, pytest.raises(ApplicationError) as captured:
            JobService(session).accept(
                owner_id=owner_id,
                command=plan_keyword_discovery(changed)[0],
            )
        assert captured.value.code == "idempotency_conflict"

        with Session(engine) as session:
            messages = session.scalars(
                select(OutboxMessage).where(
                    OutboxMessage.aggregate_id.in_([job.id for job in accepted])
                )
            ).all()
            assert len(messages) == 2
    finally:
        with engine.begin() as connection:
            connection.execute(text("DELETE FROM identity_users WHERE id = :id"), {"id": owner_id})
        engine.dispose()
