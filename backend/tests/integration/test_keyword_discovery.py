from __future__ import annotations

import hashlib
import json
import os
from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, select, text
from sqlalchemy.orm import Session, sessionmaker

from content.discovery import (
    KeywordDiscoveryPageCommitService,
    KeywordRequestMeter,
    plan_keyword_discovery,
)
from content.discovery_execution import KeywordDiscoveryExecutor
from content.models import ContentDiscovery, ContentRecord
from content.schemas import KeywordDiscoveryRunInput
from core.errors import ApplicationError
from jobs.cursor import plan_cursor_request
from jobs.execution import CheckpointConflictError, JobExecutionService
from jobs.models import CoverageWindow, Job, OutboxMessage
from jobs.schemas import (
    BudgetMetric,
    BudgetPolicyInput,
    BudgetScopeKind,
    ComponentPolicyInput,
    CostClass,
    CoverageWindowInput,
    JobAcceptedMessage,
    JobStatus,
)
from jobs.services import JobService, ResourceBudgetService
from sources.contracts import (
    SearchRequest,
    SourceCapability,
    SourcePage,
    SourcePageState,
    SourcePost,
    SourceSort,
    SourceStopReason,
)


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
        connection_id=uuid4(),
        connection_version=1,
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


def _post(
    identifier: str,
    published_at: datetime,
    *,
    url: str | None = None,
    text_scope: str = "full",
) -> SourcePost:
    return SourcePost(
        source_key="x",
        external_id=identifier,
        author_external_id="author-1",
        published_at=published_at,
        text=f"product fault {identifier}",
        language="en",
        like_count=0,
        comment_count=0,
        repost_count=0,
        canonical_url=url or f"https://example.invalid/posts/{identifier}",
        conversation_external_id=None,
        parent_external_id=None,
        quote_external_id=None,
        repost_external_id=None,
        text_scope=text_scope,
    )


def _page(
    now: datetime,
    state: SourcePageState,
    posts: tuple[SourcePost, ...],
    token: str | None = None,
) -> SourcePage:
    return SourcePage(
        source_key="x",
        capability=SourceCapability.SEARCH,
        state=state,
        items=posts,
        next_page_token=token,
        watermark=None,
        stop_reason=(
            SourceStopReason.END_OF_RESULTS if state is SourcePageState.COMPLETE else None
        ),
        observed_at=now,
        request_count=1,
        adapter_version="controlled-1",
    )


def _accepted_message(engine: Engine, job_id: UUID) -> JobAcceptedMessage:
    with Session(engine) as session:
        outbox = session.scalar(select(OutboxMessage).where(OutboxMessage.aggregate_id == job_id))
        assert outbox is not None
        return JobAcceptedMessage.model_validate(
            {
                "schema_version": 2,
                "message_id": outbox.id,
                "event_type": "job.accepted.v2",
                **outbox.payload,
            }
        )


def test_pages_atomically_save_distinct_channel_discoveries_and_unverified_gap() -> None:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    engine = create_engine(database_url)
    owner_id, connection_id, policy_id, retention_id = uuid4(), uuid4(), uuid4(), uuid4()
    now = datetime.now(UTC).replace(microsecond=0)
    start, end = now - timedelta(hours=3), now
    run = KeywordDiscoveryRunInput(
        run_id=uuid4(),
        configuration_ref="topic:controlled",
        configuration_version=3,
        source_key="x",
        connection_id=connection_id,
        connection_version=1,
        primary_query="product fault",
        starts_at=start,
        ends_at=end,
        page_size=20,
        latest_max_pages=3,
        latest_max_requests=6,
        top_max_pages=1,
        top_max_requests=4,
    )
    commands = plan_keyword_discovery(run)
    target_hash = hashlib.sha256(f"{run.configuration_ref}\0{run.primary_query}".encode()).digest()
    try:
        with Session(engine) as session, session.begin():
            session.execute(
                text(
                    "INSERT INTO identity_users "
                    "(id, username, password_hash, credential_version, created_at, updated_at) "
                    "VALUES (:id, :username, 'test-only-hash', 1, :now, :now)"
                ),
                {"id": owner_id, "username": f"discovery-pages-{owner_id}", "now": now},
            )
            session.execute(
                text(
                    "INSERT INTO source_connections "
                    "(id, owner_id, source_key, status, current_version, created_at, updated_at) "
                    "VALUES (:id, :owner_id, 'x', 'active', 1, :now, :now)"
                ),
                {"id": connection_id, "owner_id": owner_id, "now": now},
            )
            session.execute(
                text(
                    "INSERT INTO source_connection_versions "
                    "(connection_id, version, owner_id, secret_ref, created_by, created_at) "
                    "VALUES (:id, 1, :owner_id, 'env:CONTROLLED_FIXTURE', :owner_id, :now)"
                ),
                {"id": connection_id, "owner_id": owner_id, "now": now},
            )
            session.execute(
                text(
                    "INSERT INTO source_access_policies "
                    "(id, owner_id, source_key, capability, status, enabled, access_basis, "
                    "terms_reference, processing_purpose, component_name, component_version, "
                    "component_license, field_purposes, reviewed_at, policy_version, "
                    "created_at, updated_at) VALUES "
                    "(:id, :owner_id, 'x', 'search', 'approved', true, 'official_api', "
                    "'https://example.invalid/controlled-terms', '受控作品验证', "
                    "'controlled-collector', '1', 'MIT', CAST(:fields AS jsonb), "
                    ":now, 1, :now, :now)"
                ),
                {
                    "id": policy_id,
                    "owner_id": owner_id,
                    "now": now,
                    "fields": json.dumps(
                        {
                            name: "受控资料字段"
                            for name in (
                                "object_type",
                                "external_id",
                                "canonical_url",
                                "author_external_id",
                                "published_at",
                                "like_count",
                                "comment_count",
                                "repost_count",
                                "text_scope",
                                "text_origin",
                                "body",
                                "truncation_reason",
                            )
                        },
                        ensure_ascii=False,
                    ),
                },
            )
            session.execute(
                text(
                    "INSERT INTO evidence_retention_policies "
                    "(id, owner_id, source_policy_id, source_policy_version, data_class, "
                    "requested_days, source_max_days, effective_days, policy_version, "
                    "created_at, updated_at) VALUES "
                    "(:id, :owner_id, :policy_id, 1, 'structured', 30, NULL, 30, 1, :now, :now)"
                ),
                {"id": retention_id, "owner_id": owner_id, "policy_id": policy_id, "now": now},
            )
            accepted = [
                JobService(session, clock=lambda: now).accept_in_transaction(
                    owner_id=owner_id, command=command
                )
                for command in commands
            ]

        windows = {
            sort: CoverageWindowInput(
                owner_id=owner_id,
                source_key="x",
                capability=SourceCapability.SEARCH,
                target_hash=target_hash,
                sort_key=sort,
                rule_version=3,
                starts_at=start,
                ends_at=end,
            )
            for sort in (SourceSort.LATEST, SourceSort.TOP)
        }
        latest_id, top_id = (job.id for job in accepted)
        with Session(engine) as session:
            latest_lease = JobExecutionService(
                session, lease_seconds=60, clock=lambda: now
            ).acquire(job_id=latest_id, worker_id="controlled-latest")
        with Session(engine) as session:
            budget = ResourceBudgetService(session, clock=lambda: now)
            budget.save_component_policy(
                owner_id=owner_id,
                command=ComponentPolicyInput(
                    component_key="collector.controlled",
                    component_version="1",
                    cost_class=CostClass.LOCAL,
                    enabled_for_core=True,
                    terms_reference="https://example.invalid/controlled-terms",
                    reviewed_at=now,
                ),
            )
            budget_policy = BudgetPolicyInput(
                budget_key="global.controlled-requests",
                metric=BudgetMetric.NETWORK_REQUEST,
                scope_kind=BudgetScopeKind.GLOBAL,
                scope_reference=None,
                limit_units=2,
                window_seconds=60,
                window_anchor_at=now,
                enabled=True,
            )
            budget.save_budget_policy(owner_id=owner_id, command=budget_policy)
        first_request = plan_cursor_request(
            window=windows[SourceSort.LATEST],
            checkpoint=latest_lease.checkpoint,
            live_token=None,
            max_pages=3,
            max_rescans=1,
        )
        old = _post("old", start - timedelta(minutes=1))
        a = _post("a", start + timedelta(minutes=1))
        invalid = _post("invalid", start + timedelta(minutes=2), url="not-a-url")
        with Session(engine) as session, pytest.raises(ValueError):
            KeywordDiscoveryPageCommitService(
                session, lease_seconds=60, clock=lambda: now
            ).commit_page(
                owner_id=owner_id,
                lease=latest_lease,
                window=windows[SourceSort.LATEST],
                request=first_request,
                page_operation_id=uuid4(),
                connection_id=connection_id,
                connection_version=1,
                page=_page(now, SourcePageState.MORE, (old, a, invalid), "cursor-1"),
            )
        with Session(engine) as session:
            assert (
                session.scalar(select(ContentRecord).where(ContentRecord.owner_id == owner_id))
                is None
            )
            assert (
                session.scalar(select(CoverageWindow).where(CoverageWindow.owner_id == owner_id))
                is None
            )
            assert session.get(Job, latest_id).checkpoint_sequence == 0

        with Session(engine) as session:
            meter = KeywordRequestMeter(
                session,
                owner_id=owner_id,
                lease=latest_lease,
                operation_id=commands[0].operation_id,
                source_key="x",
                connection_id=connection_id,
                connection_version=1,
                component_key="collector.controlled",
                max_requests=3,
                lease_seconds=60,
                clock=lambda: now,
            )
            assert meter.before_request(1)
            assert meter.before_request(2)
            assert not meter.before_request(3)
            with engine.connect() as connection:
                assert (
                    connection.execute(
                        text("SELECT requests_sent FROM jobs WHERE id = :id"), {"id": latest_id}
                    ).scalar_one()
                    == 2
                )
                assert (
                    connection.execute(
                        text(
                            "SELECT count(*) FROM resource_usage_attempts WHERE outcome = 'started'"
                        )
                    ).scalar_one()
                    == 2
                )
            with pytest.raises(ValueError, match="usage does not match"):
                KeywordDiscoveryPageCommitService(
                    session, lease_seconds=60, clock=lambda: now
                ).commit_page(
                    owner_id=owner_id,
                    lease=latest_lease,
                    window=windows[SourceSort.LATEST],
                    request=first_request,
                    page_operation_id=uuid4(),
                    connection_id=connection_id,
                    connection_version=1,
                    page=_page(now, SourcePageState.MORE, (old, a), "cursor-1"),
                    meter=meter,
                )
            first = KeywordDiscoveryPageCommitService(
                session, lease_seconds=60, clock=lambda: now
            ).commit_page(
                owner_id=owner_id,
                lease=latest_lease,
                window=windows[SourceSort.LATEST],
                request=first_request,
                page_operation_id=uuid4(),
                connection_id=connection_id,
                connection_version=1,
                page=_page(now, SourcePageState.MORE, (old, a), "cursor-1").model_copy(
                    update={"request_count": 2}
                ),
                meter=meter,
            )
        with engine.connect() as connection:
            assert (
                connection.execute(
                    text("SELECT count(*) FROM resource_usage_attempts WHERE outcome = 'succeeded'")
                ).scalar_one()
                == 2
            )
            assert (
                connection.execute(
                    text("SELECT used_units FROM resource_budget_windows")
                ).scalar_one()
                == 2
            )
        with Session(engine) as session:
            ResourceBudgetService(session, clock=lambda: now).save_budget_policy(
                owner_id=owner_id,
                command=budget_policy.model_copy(update={"limit_units": 6}),
            )
            resumed = KeywordRequestMeter(
                session,
                owner_id=owner_id,
                lease=first.lease,
                operation_id=commands[0].operation_id,
                source_key="x",
                connection_id=connection_id,
                connection_version=1,
                component_key="collector.controlled",
                max_requests=2,
                lease_seconds=60,
                clock=lambda: now,
            )
            assert not resumed.before_request(1)
        assert first.saved_items == 1
        assert first.filtered_items == 1
        assert first.coverage.status == "running"
        with Session(engine) as session, pytest.raises(CheckpointConflictError):
            KeywordDiscoveryPageCommitService(
                session, lease_seconds=60, clock=lambda: now
            ).commit_page(
                owner_id=owner_id,
                lease=latest_lease,
                window=windows[SourceSort.LATEST],
                request=first_request,
                page_operation_id=uuid4(),
                connection_id=connection_id,
                connection_version=1,
                page=_page(now, SourcePageState.MORE, (a,), "cursor-1"),
            )
        b = _post("b", start + timedelta(minutes=3), text_scope="truncated")
        submitted_latest: list[SearchRequest] = []

        class ControlledLatestAdapter:
            source_key = "x"
            capabilities = frozenset({SourceCapability.SEARCH})

            def __init__(self, before_request: Callable[[int], bool]) -> None:
                self._before_request = before_request

            def fetch_page(self, request: SearchRequest) -> SourcePage:
                submitted_latest.append(request)
                assert self._before_request(len(submitted_latest))
                if len(submitted_latest) == 1:
                    return _page(now, SourcePageState.MORE, (old, a), "cursor-1")
                return _page(now, SourcePageState.COMPLETE, (a, b))

        latest_renewed, latest_completion = KeywordDiscoveryExecutor(
            sessionmaker(bind=engine),
            lease_seconds=60,
            component_key="collector.controlled",
            adapter_factory=lambda before, _cancelled, _limit: ControlledLatestAdapter(before),
            clock=lambda: now,
        ).execute(_accepted_message(engine, latest_id), first.lease)
        assert latest_renewed.checkpoint_sequence == 3
        assert latest_completion.status is JobStatus.PARTIALLY_SUCCEEDED
        assert [(request.sort, request.page_token) for request in submitted_latest] == [
            (SourceSort.LATEST, None),
            (SourceSort.LATEST, "cursor-1"),
        ]
        with Session(engine) as session:
            latest_coverage = session.scalar(
                select(CoverageWindow).where(
                    CoverageWindow.owner_id == owner_id,
                    CoverageWindow.sort_key == SourceSort.LATEST.value,
                )
            )
            assert latest_coverage is not None
            assert (latest_coverage.status, latest_coverage.stop_reason) == (
                "partial",
                "unverified_terminal",
            )

        with Session(engine) as session:
            top_lease = JobExecutionService(session, lease_seconds=60, clock=lambda: now).acquire(
                job_id=top_id, worker_id="controlled-top"
            )
        top_request = plan_cursor_request(
            window=windows[SourceSort.TOP],
            checkpoint=top_lease.checkpoint,
            live_token=None,
            max_pages=1,
            max_rescans=1,
        )
        c = _post("c", start + timedelta(minutes=4))
        with engine.begin() as connection:
            connection.execute(
                text("UPDATE source_connections SET status = 'disabled' WHERE id = :id"),
                {"id": connection_id},
            )
        with Session(engine) as session, pytest.raises(ApplicationError) as captured:
            KeywordDiscoveryPageCommitService(
                session, lease_seconds=60, clock=lambda: now
            ).commit_page(
                owner_id=owner_id,
                lease=top_lease,
                window=windows[SourceSort.TOP],
                request=top_request,
                page_operation_id=uuid4(),
                connection_id=connection_id,
                connection_version=1,
                page=_page(now, SourcePageState.MORE, (a, c), "top-cursor"),
            )
        assert captured.value.code == "connection_disabled"
        with Session(engine) as session, pytest.raises(ApplicationError) as captured:
            KeywordRequestMeter(
                session,
                owner_id=owner_id,
                lease=top_lease,
                operation_id=commands[1].operation_id,
                source_key="x",
                connection_id=connection_id,
                connection_version=1,
                component_key="collector.controlled",
                max_requests=4,
                lease_seconds=60,
                clock=lambda: now,
            ).before_request(1)
        assert captured.value.code == "connection_disabled"
        with engine.connect() as connection:
            assert (
                connection.execute(
                    text("SELECT requests_sent FROM jobs WHERE id = :id"), {"id": top_id}
                ).scalar_one()
                == 0
            )
        with engine.begin() as connection:
            connection.execute(
                text("UPDATE source_connections SET status = 'active' WHERE id = :id"),
                {"id": connection_id},
            )

        class CrashingSearchAdapter:
            source_key = "x"
            capabilities = frozenset({SourceCapability.SEARCH})

            def __init__(self, before_request: Callable[[int], bool]) -> None:
                self._before_request = before_request

            def fetch_page(self, _request: SearchRequest) -> SourcePage:
                assert self._before_request(1)
                raise RuntimeError("controlled transport failure")

        with pytest.raises(RuntimeError, match="controlled transport failure"):
            KeywordDiscoveryExecutor(
                sessionmaker(bind=engine),
                lease_seconds=60,
                component_key="collector.controlled",
                adapter_factory=lambda before, _cancelled, _limit: CrashingSearchAdapter(before),
                clock=lambda: now,
            ).execute(_accepted_message(engine, top_id), top_lease)
        with engine.connect() as connection:
            assert (
                connection.execute(
                    text("SELECT count(*) FROM resource_usage_attempts WHERE outcome = 'failed'")
                ).scalar_one()
                == 1
            )
            assert (
                connection.execute(
                    text("SELECT count(*) FROM coverage_windows WHERE sort_key = 'top'")
                ).scalar_one()
                == 0
            )
        submitted: list[SearchRequest] = []

        class ControlledSearchAdapter:
            source_key = "x"
            capabilities = frozenset({SourceCapability.SEARCH})

            def __init__(self, before_request: Callable[[int], bool]) -> None:
                self._before_request = before_request

            def fetch_page(self, request: SearchRequest) -> SourcePage:
                submitted.append(request)
                assert self._before_request(1)
                return _page(now, SourcePageState.MORE, (a, c), "top-cursor")

        top_renewed, completion = KeywordDiscoveryExecutor(
            sessionmaker(bind=engine),
            lease_seconds=60,
            component_key="collector.controlled",
            adapter_factory=lambda before, _cancelled, _limit: ControlledSearchAdapter(before),
            clock=lambda: now,
        ).execute(_accepted_message(engine, top_id), top_lease)
        assert top_renewed.checkpoint_sequence == 1
        assert completion.status is JobStatus.PARTIALLY_SUCCEEDED
        assert completion.failure is not None
        assert completion.failure.error_code == "search_scope_incomplete"
        assert [(request.query, request.sort, request.page_token) for request in submitted] == [
            ("product fault", SourceSort.TOP, None)
        ]
        with Session(engine) as session:
            records = session.scalars(
                select(ContentRecord).where(ContentRecord.owner_id == owner_id)
            ).all()
            discoveries = session.scalars(
                select(ContentDiscovery).where(ContentDiscovery.owner_id == owner_id)
            ).all()
            assert len(records) == 3
            assert len(discoveries) == 4
            assert {item.job_id for item in discoveries} == {latest_id, top_id}
            assert session.get(Job, latest_id).items_saved == 2
            assert session.get(Job, top_id).items_saved == 2
            assert "cursor-1" not in json.dumps(session.get(Job, latest_id).checkpoint)
            top_coverage = session.scalar(
                select(CoverageWindow).where(
                    CoverageWindow.owner_id == owner_id,
                    CoverageWindow.sort_key == SourceSort.TOP.value,
                )
            )
            assert top_coverage is not None
            assert (top_coverage.status, top_coverage.stop_reason) == (
                "partial",
                "budget_exhausted",
            )
    finally:
        with engine.begin() as connection:
            connection.execute(text("SET CONSTRAINTS ALL DEFERRED"))
            connection.execute(
                text("DELETE FROM content_records WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            )
            connection.execute(
                text("DELETE FROM source_connection_versions WHERE owner_id = :owner_id"),
                {"owner_id": owner_id},
            )
            connection.execute(text("DELETE FROM identity_users WHERE id = :id"), {"id": owner_id})
        engine.dispose()
