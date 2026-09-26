from __future__ import annotations

import hashlib
import json
import os
from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, select, text
from sqlalchemy.orm import Session, sessionmaker

from connections.presets import BILIBILI_PRESET
from content.discovery import (
    KeywordDiscoveryPageCommitService,
    KeywordRequestMeter,
    plan_keyword_discovery,
)
from content.discovery_execution import KeywordDiscoveryExecutor
from content.models import ContentDiscovery, ContentRecord
from content.schemas import KeywordDiscoveryRunInput
from content.services import ContentService
from core.errors import ApplicationError
from jobs.cursor import plan_cursor_request
from jobs.execution import (
    CheckpointConflictError,
    JobExecutionFailure,
    JobExecutionService,
    MessageReference,
)
from jobs.models import CoverageWindow, Job, OutboxMessage
from jobs.schemas import (
    BudgetMetric,
    BudgetPolicyInput,
    BudgetScopeKind,
    ComponentPolicyInput,
    CostClass,
    CoverageWindowInput,
    JobAcceptedMessage,
    JobFailureCategory,
    JobRetryScheduledMessage,
    JobStatus,
)
from jobs.services import JobService, ResourceBudgetService
from monitors.services import MonitorTopicService, evaluate_monitor_rules
from sources.adapters.mediacrawler import MediaCrawlerAdapter
from sources.contracts import (
    CommentsRequest,
    SearchRequest,
    SourceCapability,
    SourcePage,
    SourcePageState,
    SourcePost,
    SourceSort,
    SourceStopReason,
)


def test_job_execution_failure_allows_traceback_assignment() -> None:
    failure = JobExecutionFailure(
        error_code="search_policy_unavailable",
        category=JobFailureCategory.CONFIGURATION_UNAVAILABLE,
        occurred_at=datetime.now(UTC),
        next_action="配置预算政策后重试",
    )
    failure.__traceback__ = None


def test_bilibili_saved_search_output_commits_posts_and_cached_comments(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Replay the five-video/two-comment JSONL shape without launching a browser."""
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    engine = create_engine(database_url)
    owner_id, connection_id, topic_id, policy_id, retention_id = (uuid4() for _ in range(5))
    now = datetime.now(UTC)
    start, end = now - timedelta(days=1), now
    videos = (
        ("117335704734360", "Claude DeepSeek", "公开视频简介", 1790401149),
        ("117335721579544", "DeepSeek 折叠屏", "", 1790401063),
        ("117335687959822", "DeepSeek 小说", "-", 1790400547),
        ("117335688023302", "deepseek 心得", "公开视频正文", 1790400534),
        ("117335671180288", "万众瞩目大肥鱼", "公开视频描述", 1790400393),
    )
    comments = (
        (videos[0][0], "318652179920"),
        (videos[1][0], "318651673424"),
    )
    output = tmp_path / "output"
    crawler = tmp_path / "crawler"
    crawler.mkdir()
    run = KeywordDiscoveryRunInput(
        run_id=uuid4(),
        configuration_ref=f"topic:{topic_id}",
        configuration_version=1,
        source_key="bilibili",
        connection_id=connection_id,
        connection_version=1,
        primary_query="DeepSeek",
        starts_at=start,
        ends_at=end,
        page_size=5,
        latest_max_pages=1,
        latest_max_requests=26,
        top_max_pages=1,
        top_max_requests=26,
        max_seconds=220,
    )
    command = plan_keyword_discovery(run)[0]

    def replay(_self: MediaCrawlerAdapter, _request: SearchRequest, run_dir: Path) -> None:
        jsonl = run_dir / "bili" / "jsonl"
        jsonl.mkdir(parents=True)
        (jsonl / "search_contents_fixture.jsonl").write_text(
            "".join(
                json.dumps(
                    {
                        "video_id": video_id,
                        "video_url": f"https://www.bilibili.com/video/av{video_id}",
                        "title": title,
                        "desc": description,
                        "create_time": int(now.timestamp()) - (1790401149 - published),
                        "creator_hash": f"author-{video_id}",
                        "nickname": "作者",
                        "liked_count": "1",
                        "video_play_count": "2",
                        "video_comment": "1",
                        "video_share_count": "0",
                    },
                    ensure_ascii=False,
                )
                + "\n"
                for video_id, title, description, published in videos
            ),
            encoding="utf-8",
        )
        (jsonl / "search_comments_fixture.jsonl").write_text(
            "".join(
                json.dumps(
                    {
                        "video_id": video_id,
                        "comment_id": comment_id,
                        "parent_comment_id": "0",
                        "create_time": int(now.timestamp()),
                        "content": "一级评论",
                        "like_count": 0,
                    },
                    ensure_ascii=False,
                )
                + "\n"
                for video_id, comment_id in comments
            ),
            encoding="utf-8",
        )
        return None

    monkeypatch.setattr(MediaCrawlerAdapter, "_validate_crawler", lambda _self: None)
    monkeypatch.setattr(MediaCrawlerAdapter, "_run_child", replay)
    try:
        with Session(engine) as session, session.begin():
            session.execute(
                text(
                    "INSERT INTO identity_users (id, username, password_hash, credential_version, "
                    "created_at, updated_at) VALUES "
                    "(:id, :username, 'test-only-hash', 1, :now, :now)"
                ),
                {"id": owner_id, "username": f"bilibili-{owner_id}", "now": now},
            )
            session.execute(
                text(
                    "INSERT INTO monitor_topics (id, owner_id, name, status, readiness_status, "
                    "current_version, created_at, updated_at) VALUES "
                    "(:id, :owner_id, 'Bilibili replay', 'paused', 'pending_source_selection', "
                    "1, :now, :now)"
                ),
                {"id": topic_id, "owner_id": owner_id, "now": now},
            )
            session.execute(
                text(
                    "INSERT INTO monitor_topic_versions (topic_id, version, created_by, "
                    "match_any, match_all, exclude, created_at) VALUES "
                    "(:id, 1, :owner_id, CAST(:terms AS jsonb), '[]', '[]', :now)"
                ),
                {
                    "id": topic_id,
                    "owner_id": owner_id,
                    "now": now,
                    "terms": json.dumps(["DeepSeek", "大肥鱼"]),
                },
            )
            session.execute(
                text(
                    "INSERT INTO source_connections (id, owner_id, source_key, status, "
                    "current_version, created_at, updated_at) VALUES "
                    "(:id, :owner_id, 'bilibili', 'active', 1, :now, :now)"
                ),
                {"id": connection_id, "owner_id": owner_id, "now": now},
            )
            session.execute(
                text(
                    "INSERT INTO source_connection_versions (connection_id, version, owner_id, "
                    "secret_ref, created_by, created_at) VALUES "
                    "(:id, 1, :owner_id, 'env:CONTROLLED_FIXTURE', :owner_id, :now)"
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
                    "(:id, :owner_id, 'bilibili', 'search', 'approved', true, 'public_web', "
                    "'https://example.invalid/terms', '受控重放', 'collector.bilibili', '1', "
                    "'test', CAST(:fields AS jsonb), :now, 1, :now, :now)"
                ),
                {
                    "id": policy_id,
                    "owner_id": owner_id,
                    "now": now,
                    "fields": json.dumps(
                        dict(BILIBILI_PRESET.capabilities[0].field_purposes), ensure_ascii=False
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
            accepted = JobService(session).accept_in_transaction(owner_id=owner_id, command=command)
        with Session(engine) as session:
            lease = JobExecutionService(session, lease_seconds=60).acquire(
                job_id=accepted.id, worker_id="bilibili-replay"
            )
        with Session(engine) as session:
            budget = ResourceBudgetService(session)
            budget.save_component_policy(
                owner_id=owner_id,
                command=ComponentPolicyInput(
                    component_key="collector.bilibili",
                    component_version="1",
                    cost_class=CostClass.LOCAL,
                    enabled_for_core=True,
                    terms_reference="https://example.invalid/terms",
                    reviewed_at=now,
                ),
            )
            budget.save_budget_policy(
                owner_id=owner_id,
                command=BudgetPolicyInput(
                    budget_key="global.bilibili-network.daily",
                    metric=BudgetMetric.NETWORK_REQUEST,
                    scope_kind=BudgetScopeKind.GLOBAL,
                    scope_reference=None,
                    limit_units=60,
                    window_seconds=86_400,
                    window_anchor_at=datetime(2026, 1, 1, tzinfo=UTC),
                    enabled=True,
                ),
            )
            budget.save_budget_policy(
                owner_id=owner_id,
                command=BudgetPolicyInput(
                    budget_key="source.bilibili.network.daily",
                    metric=BudgetMetric.NETWORK_REQUEST,
                    scope_kind=BudgetScopeKind.SOURCE,
                    scope_reference="bilibili",
                    limit_units=60,
                    window_seconds=86_400,
                    window_anchor_at=datetime(2026, 1, 1, tzinfo=UTC),
                    enabled=True,
                ),
            )

        adapter: MediaCrawlerAdapter | None = None

        def adapter_factory(
            before_request: Callable[[int], bool],
            cancelled: Callable[[], bool],
            max_requests: int,
            max_seconds: float,
        ) -> MediaCrawlerAdapter:
            nonlocal adapter
            adapter = MediaCrawlerAdapter(
                crawler_dir=crawler,
                output_dir=output,
                owner_key=owner_id.hex,
                before_request=before_request,
                cancelled=cancelled,
                max_requests=max_requests,
                max_seconds=max_seconds,
                verify_revision=False,
            )
            return adapter

        _, completion = KeywordDiscoveryExecutor(
            sessionmaker(bind=engine),
            lease_seconds=60,
            adapter_factory=adapter_factory,
        ).execute(_accepted_message(engine, accepted.id), lease)
        assert completion.status is JobStatus.PARTIALLY_SUCCEEDED
        assert adapter is not None
        with Session(engine) as session:
            records = session.scalars(
                select(ContentRecord).where(ContentRecord.owner_id == owner_id)
            ).all()
            assert {record.external_id for record in records} == {video[0] for video in videos}
            job = session.get(Job, accepted.id)
            assert job is not None
            assert (job.requests_sent, job.items_saved) == (26, 5)
            assert (
                session.execute(
                    text(
                        "SELECT count(*) FROM resource_usage_attempts WHERE operation_id = :id "
                        "AND outcome = 'failed'"
                    ),
                    {"id": command.operation_id},
                ).scalar_one()
                == 26
            )
        cached = [
            adapter.fetch_page(
                CommentsRequest(source_key="bilibili", post_external_id=video_id, page_size=20)
            )
            for video_id, _ in comments
        ]
        assert [page.request_count for page in cached] == [0, 0]
        assert [page.items[0].external_id for page in cached] == [
            comment_id for _, comment_id in comments
        ]
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
            connection.execute(
                text("DELETE FROM monitor_topic_versions WHERE topic_id = :topic_id"),
                {"topic_id": topic_id},
            )
            connection.execute(
                text("DELETE FROM monitor_topics WHERE id = :topic_id"),
                {"topic_id": topic_id},
            )
            connection.execute(text("DELETE FROM identity_users WHERE id = :id"), {"id": owner_id})
        engine.dispose()


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
        max_seconds=30,
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
    body: str | None = "product fault release",
    like_count: int = 0,
    text_scope: str = "full",
) -> SourcePost:
    return SourcePost(
        source_key="x",
        external_id=identifier,
        author_external_id="author-1",
        published_at=published_at,
        text=body,
        language="en",
        like_count=like_count,
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
    owner_id, connection_id, policy_id, retention_id, topic_id = (
        uuid4(),
        uuid4(),
        uuid4(),
        uuid4(),
        uuid4(),
    )
    now = datetime.now(UTC).replace(microsecond=0)
    start, end = now - timedelta(hours=3), now
    run = KeywordDiscoveryRunInput(
        run_id=uuid4(),
        configuration_ref=f"topic:{topic_id}",
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
        max_seconds=30,
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
                    "INSERT INTO monitor_topics "
                    "(id, owner_id, name, status, readiness_status, current_version, "
                    "created_at, updated_at) VALUES "
                    "(:id, :owner_id, 'Controlled topic', 'paused', "
                    "'pending_source_selection', 4, :now, :now)"
                ),
                {"id": topic_id, "owner_id": owner_id, "now": now},
            )
            session.execute(
                text(
                    "INSERT INTO monitor_topic_versions "
                    "(topic_id, version, created_by, match_any, match_all, exclude, created_at) "
                    "VALUES (:topic_id, 3, :owner_id, CAST(:match_any AS jsonb), "
                    "CAST(:match_all AS jsonb), CAST(:exclude AS jsonb), :now), "
                    "(:topic_id, 4, :owner_id, '[\"future version\"]', '[]', '[]', :now)"
                ),
                {
                    "topic_id": topic_id,
                    "owner_id": owner_id,
                    "match_any": json.dumps(["product fault", "hotkey"]),
                    "match_all": json.dumps(["release"]),
                    "exclude": json.dumps(["spam"]),
                    "now": now,
                },
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
        excluded = _post(
            "excluded",
            start + timedelta(minutes=3),
            body="product fault release spam",
        )
        unrelated_high = _post(
            "unrelated-high",
            start + timedelta(minutes=4),
            body="unrelated meme release",
            like_count=100_000,
        )
        missing_text = _post("missing-text", start + timedelta(minutes=5), body=None)
        zero_interaction = _post(
            "zero-interaction",
            start + timedelta(minutes=6),
            body="HOTKEY RELEASE launch",
        )
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
                deadline_at=now + timedelta(seconds=30),
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
                    update={
                        "items": (
                            old,
                            a,
                            excluded,
                            unrelated_high,
                            missing_text,
                            zero_interaction,
                        ),
                        "request_count": 2,
                    }
                ),
                meter=meter,
            )
            assert first.saved_items == 2
            assert first.filtered_items == 4
            assert first.lease.checkpoint["collection.observed_count"] == 6
            topic_job = session.get(Job, latest_id)
            assert topic_job is not None
            assert topic_job.scope["relevance_filter_position"] == "local"
            assert not {"match_any", "match_all", "exclude"}.intersection(topic_job.scope)

        with Session(engine) as session, session.begin():
            rules = MonitorTopicService(session).get_topic_rules_in_transaction(
                owner_id=owner_id,
                topic_id=topic_id,
                version=3,
            )
            assert evaluate_monitor_rules(rules, "HOTKEY release").matched
            with pytest.raises(ApplicationError) as missing_version:
                MonitorTopicService(session).get_topic_rules_in_transaction(
                    owner_id=owner_id,
                    topic_id=topic_id,
                    version=2,
                )
            assert missing_version.value.code == "resource_not_found"
            with pytest.raises(ApplicationError) as wrong_owner:
                MonitorTopicService(session).get_topic_rules_in_transaction(
                    owner_id=uuid4(),
                    topic_id=topic_id,
                    version=3,
                )
            assert wrong_owner.value.code == "resource_not_found"
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
                deadline_at=now + timedelta(seconds=30),
                lease_seconds=60,
                clock=lambda: now,
            )
            assert not resumed.before_request(1)
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
            adapter_factory=lambda before, _cancelled, _limit, _seconds: ControlledLatestAdapter(
                before
            ),
            clock=lambda: now,
        ).execute(_accepted_message(engine, latest_id), first.lease)
        assert latest_renewed.checkpoint_sequence == 3
        assert latest_completion.status is JobStatus.PARTIALLY_SUCCEEDED
        assert [(request.sort, request.page_token) for request in submitted_latest] == [
            (SourceSort.LATEST, None),
            (SourceSort.LATEST, "cursor-1"),
        ]
        assert [(request.starts_at, request.ends_at) for request in submitted_latest] == [
            (start, end),
            (start, end),
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
                deadline_at=now + timedelta(seconds=30),
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

        with pytest.raises(JobExecutionFailure) as transport_error:
            KeywordDiscoveryExecutor(
                sessionmaker(bind=engine),
                lease_seconds=60,
                component_key="collector.controlled",
                adapter_factory=lambda before, _cancelled, _limit, _seconds: CrashingSearchAdapter(
                    before
                ),
                clock=lambda: now,
            ).execute(_accepted_message(engine, top_id), top_lease)
        assert transport_error.value.error_code == "search_source_failed"
        assert transport_error.value.category is JobFailureCategory.TRANSIENT
        with engine.connect() as connection:
            assert (
                connection.execute(
                    text("SELECT count(*) FROM resource_usage_attempts WHERE outcome = 'failed'")
                ).scalar_one()
                == 1
            )
        with Session(engine) as session:
            failed_coverage = session.scalar(
                select(CoverageWindow).where(
                    CoverageWindow.owner_id == owner_id,
                    CoverageWindow.sort_key == SourceSort.TOP.value,
                )
            )
            assert failed_coverage is not None
            assert (
                failed_coverage.status,
                failed_coverage.stop_reason,
                failed_coverage.page_count,
            ) == ("partial", "upstream_error", 0)
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
            adapter_factory=lambda before, _cancelled, _limit, _seconds: ControlledSearchAdapter(
                before
            ),
            clock=lambda: now,
        ).execute(_accepted_message(engine, top_id), top_lease)
        assert top_renewed.checkpoint_sequence == 1
        assert completion.status is JobStatus.PARTIALLY_SUCCEEDED
        assert completion.failure is not None
        assert completion.failure.error_code == "search_scope_incomplete"
        assert [(request.query, request.sort, request.page_token) for request in submitted] == [
            ("product fault", SourceSort.TOP, None)
        ]
        assert [(request.starts_at, request.ends_at) for request in submitted] == [(start, end)]
        with Session(engine) as session:
            records = session.scalars(
                select(ContentRecord).where(ContentRecord.owner_id == owner_id)
            ).all()
            discoveries = session.scalars(
                select(ContentDiscovery).where(ContentDiscovery.owner_id == owner_id)
            ).all()
            assert len(records) == 4
            assert len(discoveries) == 5
            assert {item.job_id for item in discoveries} == {latest_id, top_id}
            assert session.get(Job, latest_id).items_saved == 3
            assert session.get(Job, top_id).items_saved == 2
            assert session.get(Job, latest_id).checkpoint["collection.observed_count"] == 10
            assert session.get(Job, top_id).checkpoint["collection.observed_count"] == 2
            counts = ContentService(session).collection_counts_in_transaction(
                owner_id=owner_id, job_ids=(latest_id, top_id)
            )
            assert sum(item.first_ingested_count for item in counts) == 4
            assert (
                sum(item.deduplicated_count for item in counts)
                == sum(item.observation_count for item in counts) - 4
            )
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

        with Session(engine) as session:
            ResourceBudgetService(session, clock=lambda: now).save_budget_policy(
                owner_id=owner_id,
                command=budget_policy.model_copy(update={"limit_units": 8}),
            )
            auth_run = run.model_copy(update={"run_id": uuid4(), "primary_query": "auth challenge"})
            auth_job = JobService(session, clock=lambda: now).accept(
                owner_id=owner_id, command=plan_keyword_discovery(auth_run)[0]
            )
            auth_lease = JobExecutionService(session, lease_seconds=60, clock=lambda: now).acquire(
                job_id=auth_job.id, worker_id="controlled-auth"
            )

        class ControlledStoppedAdapter:
            source_key = "x"
            capabilities = frozenset({SourceCapability.SEARCH})

            def __init__(
                self,
                before_request: Callable[[int], bool],
                reason: SourceStopReason,
                retry_at: datetime | None = None,
            ) -> None:
                self._before_request = before_request
                self._reason = reason
                self._retry_at = retry_at

            def fetch_page(self, _request: SearchRequest) -> SourcePage:
                assert self._before_request(1)
                return SourcePage(
                    source_key="x",
                    capability=SourceCapability.SEARCH,
                    state=SourcePageState.STOPPED,
                    items=(),
                    next_page_token=None,
                    watermark=None,
                    stop_reason=self._reason,
                    observed_at=now,
                    request_count=1,
                    adapter_version="controlled-1",
                    retry_at=self._retry_at,
                )

        with pytest.raises(JobExecutionFailure) as auth_error:
            KeywordDiscoveryExecutor(
                sessionmaker(bind=engine),
                lease_seconds=60,
                component_key="collector.controlled",
                adapter_factory=lambda before, _cancelled, _limit, _seconds: (
                    ControlledStoppedAdapter(before, SourceStopReason.AUTHENTICATION_REQUIRED)
                ),
                clock=lambda: now,
            ).execute(_accepted_message(engine, auth_job.id), auth_lease)
        assert auth_error.value.error_code == "source_authentication_required"
        assert auth_error.value.category is JobFailureCategory.AUTHENTICATION_REQUIRED
        with Session(engine) as session:
            auth_coverage = session.scalar(
                select(CoverageWindow).where(
                    CoverageWindow.owner_id == owner_id,
                    CoverageWindow.target_hash
                    == hashlib.sha256(f"{run.configuration_ref}\0auth challenge".encode()).digest(),
                )
            )
            assert auth_coverage is not None
            assert (auth_coverage.status, auth_coverage.stop_reason, auth_coverage.page_count) == (
                "partial",
                "authentication_required",
                1,
            )
            assert session.get(Job, auth_job.id).requests_sent == 1

        with Session(engine) as session:
            rate_run = run.model_copy(
                update={"run_id": uuid4(), "primary_query": "rate challenge", "max_seconds": 90}
            )
            rate_job = JobService(session, clock=lambda: now).accept(
                owner_id=owner_id, command=plan_keyword_discovery(rate_run)[0]
            )
            rate_lease = JobExecutionService(session, lease_seconds=60, clock=lambda: now).acquire(
                job_id=rate_job.id, worker_id="controlled-rate"
            )
        with pytest.raises(JobExecutionFailure) as rate_error:
            KeywordDiscoveryExecutor(
                sessionmaker(bind=engine),
                lease_seconds=60,
                component_key="collector.controlled",
                adapter_factory=lambda before, _cancelled, _limit, _seconds: (
                    ControlledStoppedAdapter(
                        before, SourceStopReason.RATE_LIMITED, now + timedelta(seconds=60)
                    )
                ),
                clock=lambda: now,
            ).execute(_accepted_message(engine, rate_job.id), rate_lease)
        assert rate_error.value.error_code == "source_rate_limited"
        assert rate_error.value.category is JobFailureCategory.RATE_LIMITED
        assert rate_error.value.retry_at == now + timedelta(seconds=60)
        assert rate_error.value.max_attempts == 3
        with Session(engine) as session:
            JobExecutionService(session, lease_seconds=60, clock=lambda: now).record_failure(
                rate_lease,
                message=MessageReference(
                    message_id=_accepted_message(engine, rate_job.id).message_id,
                    topic="hotkey.jobs.accepted.v2",
                    partition=0,
                    offset=37,
                ),
                failure=rate_error.value,
            )
            rate_state = session.get(Job, rate_job.id)
            assert rate_state is not None and rate_state.status == "queued"
            retry_outbox = session.scalar(
                select(OutboxMessage).where(
                    OutboxMessage.aggregate_id == rate_job.id,
                    OutboxMessage.dispatch_sequence == 2,
                )
            )
            assert retry_outbox is not None
            retry_message = JobRetryScheduledMessage.model_validate(
                {
                    "schema_version": 1,
                    "message_id": retry_outbox.id,
                    "event_type": retry_outbox.event_type,
                    **retry_outbox.payload,
                }
            )
            retry_lease = JobExecutionService(
                session, lease_seconds=60, clock=lambda: now + timedelta(seconds=60)
            ).acquire(job_id=rate_job.id, worker_id="controlled-rate-retry")

        class RateRecoveredAdapter:
            source_key = "x"
            capabilities = frozenset({SourceCapability.SEARCH})

            def __init__(self, before_request: Callable[[int], bool]) -> None:
                self._before_request = before_request

            def fetch_page(self, request: SearchRequest) -> SourcePage:
                assert request.page_token is None
                assert self._before_request(1)
                return _page(
                    now + timedelta(seconds=60),
                    SourcePageState.COMPLETE,
                    (_post("rate-recovered", start + timedelta(minutes=5)),),
                )

        resumed_lease, resumed_completion = KeywordDiscoveryExecutor(
            sessionmaker(bind=engine),
            lease_seconds=60,
            component_key="collector.controlled",
            adapter_factory=lambda before, _cancelled, _limit, remaining: (
                RateRecoveredAdapter(before)
                if 0 < remaining <= 30
                else pytest.fail("retry must receive only its remaining time budget")
            ),
            clock=lambda: now + timedelta(seconds=60),
        ).execute(retry_message, retry_lease)
        assert resumed_lease.checkpoint_sequence == 2
        assert resumed_completion.status is JobStatus.PARTIALLY_SUCCEEDED
        with Session(engine) as session:
            rate_state = session.get(Job, rate_job.id)
            assert rate_state is not None
            assert (rate_state.requests_sent, rate_state.items_saved) == (2, 1)

        with Session(engine) as session:
            deadline_run = run.model_copy(
                update={
                    "run_id": uuid4(),
                    "primary_query": "deadline query",
                    "max_seconds": 1,
                }
            )
            deadline_job = JobService(session, clock=lambda: now).accept(
                owner_id=owner_id, command=plan_keyword_discovery(deadline_run)[0]
            )
            deadline_lease = JobExecutionService(
                session, lease_seconds=60, clock=lambda: now
            ).acquire(job_id=deadline_job.id, worker_id="controlled-deadline")
        expired_lease, expired = KeywordDiscoveryExecutor(
            sessionmaker(bind=engine),
            lease_seconds=60,
            component_key="collector.controlled",
            adapter_factory=lambda _before, _cancelled, _limit, _seconds: pytest.fail(
                "expired search must not construct a source adapter"
            ),
            clock=lambda: now + timedelta(seconds=2),
        ).execute(_accepted_message(engine, deadline_job.id), deadline_lease)
        assert expired_lease.checkpoint_sequence == 0
        assert expired.status is JobStatus.PARTIALLY_SUCCEEDED
        assert expired.failure is not None
        assert expired.failure.error_code == "search_time_budget_exhausted"
        with Session(engine) as session:
            deadline_coverage = session.scalar(
                select(CoverageWindow).where(
                    CoverageWindow.owner_id == owner_id,
                    CoverageWindow.target_hash
                    == hashlib.sha256(f"{run.configuration_ref}\0deadline query".encode()).digest(),
                )
            )
            assert deadline_coverage is not None
            assert (
                deadline_coverage.status,
                deadline_coverage.stop_reason,
                deadline_coverage.page_count,
            ) == ("partial", "budget_exhausted", 0)
            assert session.get(Job, deadline_job.id).requests_sent == 0

        late_at = now + timedelta(seconds=61)
        with Session(engine) as session:
            late_run = run.model_copy(
                update={
                    "run_id": uuid4(),
                    "primary_query": "late rate challenge",
                    "latest_max_pages": 1,
                }
            )
            late_job = JobService(session, clock=lambda: late_at).accept(
                owner_id=owner_id, command=plan_keyword_discovery(late_run)[0]
            )
            late_lease = JobExecutionService(
                session, lease_seconds=60, clock=lambda: late_at
            ).acquire(job_id=late_job.id, worker_id="controlled-late-rate")
        with pytest.raises(JobExecutionFailure) as late_error:
            KeywordDiscoveryExecutor(
                sessionmaker(bind=engine),
                lease_seconds=60,
                component_key="collector.controlled",
                adapter_factory=lambda before, _cancelled, _limit, _seconds: (
                    ControlledStoppedAdapter(
                        before, SourceStopReason.RATE_LIMITED, late_at + timedelta(seconds=60)
                    )
                ),
                clock=lambda: late_at,
            ).execute(_accepted_message(engine, late_job.id), late_lease)
        assert late_error.value.category is JobFailureCategory.RATE_LIMITED
        assert late_error.value.retry_at is None
        assert late_error.value.max_attempts is None
        assert late_error.value.manual_retry_allowed is False

        with Session(engine) as session:
            cancel_run = run.model_copy(
                update={"run_id": uuid4(), "primary_query": "cancel challenge"}
            )
            cancel_job = JobService(session, clock=lambda: late_at).accept(
                owner_id=owner_id, command=plan_keyword_discovery(cancel_run)[0]
            )
            cancel_lease = JobExecutionService(
                session, lease_seconds=60, clock=lambda: late_at
            ).acquire(job_id=cancel_job.id, worker_id="controlled-cancel")

        class CancelledSearchAdapter:
            source_key = "x"
            capabilities = frozenset({SourceCapability.SEARCH})

            def __init__(self, before_request: Callable[[int], bool]) -> None:
                self._before_request = before_request

            def fetch_page(self, _request: SearchRequest) -> SourcePage:
                assert self._before_request(1)
                with Session(engine) as cancel_session:
                    JobService(cancel_session, clock=lambda: late_at).request_cancel(
                        owner_id=owner_id, job_id=cancel_job.id
                    )
                return _page(
                    late_at,
                    SourcePageState.MORE,
                    (_post("cancelled", start + timedelta(minutes=6)),),
                    "cancel-cursor",
                )

        cancel_message = _accepted_message(engine, cancel_job.id)
        cancelled_lease, cancelled_completion = KeywordDiscoveryExecutor(
            sessionmaker(bind=engine),
            lease_seconds=60,
            component_key="collector.controlled",
            adapter_factory=lambda before, _cancelled, _limit, _seconds: CancelledSearchAdapter(
                before
            ),
            clock=lambda: late_at,
        ).execute(cancel_message, cancel_lease)
        with Session(engine) as session:
            JobExecutionService(session, lease_seconds=60, clock=lambda: late_at).complete(
                cancelled_lease,
                message=MessageReference(
                    message_id=cancel_message.message_id,
                    topic="hotkey.jobs.accepted.v2",
                    partition=0,
                    offset=38,
                ),
                completion=cancelled_completion,
            )
            cancel_state = session.get(Job, cancel_job.id)
            assert cancel_state is not None
            assert (cancel_state.status, cancel_state.requests_sent, cancel_state.items_saved) == (
                "cancelled",
                1,
                0,
            )
            assert (
                session.scalar(
                    select(ContentDiscovery).where(ContentDiscovery.job_id == cancel_job.id)
                )
                is None
            )
            assert (
                session.execute(
                    text(
                        "SELECT outcome FROM resource_usage_attempts "
                        "WHERE owner_id = :owner_id AND operation_id = :operation_id"
                    ),
                    {
                        "owner_id": owner_id,
                        "operation_id": plan_keyword_discovery(cancel_run)[0].operation_id,
                    },
                ).scalar_one()
                == "failed"
            )
            cancel_coverage = session.scalar(
                select(CoverageWindow).where(
                    CoverageWindow.owner_id == owner_id,
                    CoverageWindow.target_hash
                    == hashlib.sha256(
                        f"{run.configuration_ref}\0cancel challenge".encode()
                    ).digest(),
                )
            )
            assert cancel_coverage is not None
            assert (
                cancel_coverage.status,
                cancel_coverage.stop_reason,
                cancel_coverage.page_count,
            ) == ("partial", "cancelled", 0)

        progress_clock = [late_at]
        with Session(engine) as session:
            progress_run = run.model_copy(
                update={
                    "run_id": uuid4(),
                    "primary_query": "mid-run deadline",
                    "max_seconds": 1,
                }
            )
            progress_job = JobService(session, clock=lambda: progress_clock[0]).accept(
                owner_id=owner_id, command=plan_keyword_discovery(progress_run)[0]
            )
            progress_lease = JobExecutionService(
                session, lease_seconds=60, clock=lambda: progress_clock[0]
            ).acquire(job_id=progress_job.id, worker_id="controlled-mid-deadline")

        class ExpiringSearchAdapter:
            source_key = "x"
            capabilities = frozenset({SourceCapability.SEARCH})

            def __init__(self, before_request: Callable[[int], bool]) -> None:
                self._before_request = before_request

            def fetch_page(self, _request: SearchRequest) -> SourcePage:
                assert self._before_request(1)
                progress_clock[0] = late_at + timedelta(seconds=2)
                return _page(
                    late_at,
                    SourcePageState.MORE,
                    (_post("mid-deadline", start + timedelta(minutes=7)),),
                    "next-private-cursor",
                )

        progress_renewed, progress_completion = KeywordDiscoveryExecutor(
            sessionmaker(bind=engine),
            lease_seconds=60,
            component_key="collector.controlled",
            adapter_factory=lambda before, _cancelled, _limit, _seconds: ExpiringSearchAdapter(
                before
            ),
            clock=lambda: progress_clock[0],
        ).execute(_accepted_message(engine, progress_job.id), progress_lease)
        assert progress_renewed.checkpoint_sequence == 1
        assert progress_completion.status is JobStatus.PARTIALLY_SUCCEEDED
        assert progress_completion.failure is not None
        assert progress_completion.failure.error_code == "search_time_budget_exhausted"
        with Session(engine) as session:
            progress_coverage = session.scalar(
                select(CoverageWindow).where(
                    CoverageWindow.owner_id == owner_id,
                    CoverageWindow.target_hash
                    == hashlib.sha256(
                        f"{run.configuration_ref}\0mid-run deadline".encode()
                    ).digest(),
                )
            )
            assert progress_coverage is not None
            assert (
                progress_coverage.status,
                progress_coverage.stop_reason,
                progress_coverage.page_count,
            ) == ("partial", "budget_exhausted", 1)
            assert session.get(Job, progress_job.id).items_saved == 1
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
            connection.execute(
                text("DELETE FROM monitor_topic_versions WHERE topic_id = :topic_id"),
                {"topic_id": topic_id},
            )
            connection.execute(
                text("DELETE FROM monitor_topics WHERE id = :topic_id"),
                {"topic_id": topic_id},
            )
            connection.execute(text("DELETE FROM identity_users WHERE id = :id"), {"id": owner_id})
        engine.dispose()
