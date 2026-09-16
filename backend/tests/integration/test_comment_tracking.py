from datetime import timedelta
from hashlib import sha256
from uuid import uuid4

import pytest
from sqlalchemy import func, select, update

from collection.models import CollectionBudgetUsage, CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from contents.models import Content, ContentObservation
from core.clock import utcnow
from core.errors import AppError
from evidence.models import RawPage
from jobs.models import Job, Outbox
from monitors.models import MonitorMatch
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService, match_content
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


def active_monitor(database, sources: SourceService, title: str):
    service = MonitorService(database, sources)
    created = service.create_monitor(
        MonitorInput.model_validate(
            {
                "title": title,
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "budget": {"daily_requests": 1000, "content_purchase_cost": 0},
            }
        )
    )
    return service.change_state(
        created.id,
        MonitorStateChange(expected_version=created.current_version),
        "active",
    )


def search_run(database, sources: SourceService, monitor, key: str):
    return CollectionService(database, sources, evidence_configured=True).create_run(
        CollectionRunInput(
            monitor_id=monitor.id,
            expected_version=monitor.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key=key,
            policy_version="synthetic-comment-tracking-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )


def seed_root(database, run_id, monitor_version_id, external_id: str):
    now = utcnow()
    raw_page_id = uuid4()
    content_id = uuid4()
    with database.begin() as session:
        run = session.get(CollectionRun, run_id)
        assert run is not None
        session.add(
            RawPage(
                id=raw_page_id,
                run_id=run_id,
                source="bilibili",
                operation=run.operation,
                request_fingerprint=sha256(f"request:{raw_page_id}".encode()).hexdigest(),
                bucket="synthetic-evidence",
                object_key=f"raw/synthetic/{raw_page_id}.json.gz",
                payload_sha256=sha256(f"payload:{raw_page_id}".encode()).hexdigest(),
                object_sha256=sha256(f"object:{raw_page_id}".encode()).hexdigest(),
                response_bytes=10,
                object_bytes=20,
                media_type="application/json",
                observed_at=now,
                retention_until=now + timedelta(days=7),
                policy_version="synthetic-comment-tracking-v1",
                object_state="available",
                cleanup_attempts=0,
                cleanup_error_code=None,
                deleted_at=None,
            )
        )
        session.add(
            Content(
                id=content_id,
                source="bilibili",
                provider_namespace="video",
                external_id=external_id,
                kind="post",
                canonical_url="https://www.bilibili.com/video/synthetic",
                author_ref="author:synthetic",
                root_external_id=external_id,
                parent_external_id=None,
                relation_status="root",
                visibility="available",
                first_seen_at=now,
                last_seen_at=now,
            )
        )
        session.flush()
        session.add(
            ContentObservation(
                id=uuid4(),
                content_id=content_id,
                observed_at=now,
                reply_count=3,
                raw_page_id=raw_page_id,
            )
        )
        session.flush()
        match_content(session, monitor_version_id, content_id, ["AI"], now)
        match_id = session.scalar(
            select(MonitorMatch.id).where(
                MonitorMatch.monitor_version_id == monitor_version_id,
                MonitorMatch.content_id == content_id,
            )
        )
        assert match_id is not None
    return match_id, content_id


def test_selected_content_creates_and_replays_the_expected_followup(database):
    sources = AdmittedSources()
    monitor = active_monitor(database, sources, "用户选择评论链")
    parent = search_run(database, sources, monitor, "manual-comment-tracking-search")
    sampled_match_id, _ = seed_root(
        database,
        parent.id,
        parent.monitor_version_id,
        "bvid:BV1BVFWeHEaV",
    )
    match_id, content_id = seed_root(
        database,
        parent.id,
        parent.monitor_version_id,
        "bvid:BV1BVFWeHEaW",
    )
    service = CollectionService(database, sources, evidence_configured=True)

    sampled = service.start_comment_tracking(sampled_match_id)
    assert sampled.run.operation == "fetch_post"
    assert sampled.run.request_value == "bvid:BV1BVFWeHEaV"
    detail = service.start_comment_tracking(match_id)
    assert detail.content_id == content_id
    assert detail.root_content_id == content_id
    assert detail.review_state == "following"
    assert detail.replayed is False
    assert detail.run.parent_run_id == parent.id
    assert detail.run.operation == "fetch_post"
    assert detail.run.request_value == "bvid:BV1BVFWeHEaW"
    assert detail.run.id != sampled.run.id

    replay = service.start_comment_tracking(match_id)
    assert replay.replayed is True
    assert replay.run.id == detail.run.id
    with database() as session:
        assert session.get(MonitorMatch, match_id).review_state == "following"
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 3
        assert session.scalar(select(func.count()).select_from(Job)) == 3
        assert session.scalar(select(func.count()).select_from(Outbox)) == 3
        usage = session.scalar(select(CollectionBudgetUsage))
        assert usage is not None
        assert usage.reserved_requests == 3

    comments_match_id, comments_content_id = seed_root(
        database,
        detail.run.id,
        parent.monitor_version_id,
        "aid:200",
    )
    comments = service.start_comment_tracking(comments_match_id)
    assert comments.content_id == comments_content_id
    assert comments.run.parent_run_id == detail.run.id
    assert comments.run.operation == "list_comments"
    assert comments.run.request_value == "aid:200"
    assert comments.replayed is False
    comments_replay = service.start_comment_tracking(comments_match_id)
    assert comments_replay.replayed is True
    assert comments_replay.run.id == comments.run.id


def test_comment_tracking_failures_leave_match_budget_and_queue_unchanged(database):
    sources = AdmittedSources()
    monitor = active_monitor(database, sources, "评论追踪失败边界")
    parent = search_run(database, sources, monitor, "comment-tracking-failure-search")
    match_id, _ = seed_root(
        database,
        parent.id,
        parent.monitor_version_id,
        "bvid:BV1BVFWeHEaV",
    )

    with pytest.raises(AppError, match="evidence_store_not_configured"):
        CollectionService(database, sources).start_comment_tracking(match_id)
    with database.begin() as session:
        usage = session.scalar(select(CollectionBudgetUsage))
        assert usage is not None
        session.execute(
            update(CollectionBudgetUsage)
            .where(CollectionBudgetUsage.id == usage.id)
            .values(limit_requests=usage.reserved_requests)
        )
    with pytest.raises(AppError, match="request_budget_exhausted"):
        CollectionService(database, sources, evidence_configured=True).start_comment_tracking(
            match_id
        )

    with database() as session:
        assert session.get(MonitorMatch, match_id).review_state == "new"
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 1
        assert session.scalar(select(func.count()).select_from(Job)) == 1
        assert session.scalar(select(func.count()).select_from(Outbox)) == 1
        usage = session.scalar(select(CollectionBudgetUsage))
        assert usage is not None
        assert usage.reserved_requests == 1


def test_comment_tracking_rejects_evidence_from_another_monitor_version(database):
    sources = AdmittedSources()
    selected_monitor = active_monitor(database, sources, "被选择的监控")
    origin_monitor = active_monitor(database, sources, "证据来源监控")
    selected = search_run(database, sources, selected_monitor, "selected-version-search")
    origin = search_run(database, sources, origin_monitor, "other-version-search")
    match_id, content_id = seed_root(
        database,
        origin.id,
        selected.monitor_version_id,
        "bvid:BV1BVFWeHEaV",
    )
    with database.begin() as session:
        selected_version_id = session.scalar(
            select(MonitorMatch.monitor_version_id).where(MonitorMatch.id == match_id)
        )
        assert selected_version_id is not None
        match = session.get(MonitorMatch, match_id)
        assert match is not None and match.content_id == content_id

    with pytest.raises(AppError, match="comment_tracking_origin_missing"):
        CollectionService(database, sources, evidence_configured=True).start_comment_tracking(
            match_id
        )
    with database() as session:
        assert session.get(MonitorMatch, match_id).review_state == "new"


def test_comment_tracking_rejects_an_inactive_monitor_without_queue_writes(database):
    sources = AdmittedSources()
    monitor = active_monitor(database, sources, "已暂停监控")
    parent = search_run(database, sources, monitor, "inactive-comment-tracking-search")
    match_id, _ = seed_root(
        database,
        parent.id,
        parent.monitor_version_id,
        "bvid:BV1BVFWeHEaV",
    )
    MonitorService(database, sources).change_state(
        monitor.id,
        MonitorStateChange(expected_version=monitor.current_version),
        "paused",
    )

    with pytest.raises(AppError, match="monitor_not_active"):
        CollectionService(database, sources, evidence_configured=True).start_comment_tracking(
            match_id
        )
    with database() as session:
        assert session.get(MonitorMatch, match_id).review_state == "new"
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 1
        assert session.scalar(select(func.count()).select_from(Job)) == 1
        assert session.scalar(select(func.count()).select_from(Outbox)) == 1


def test_comment_tracking_rejects_an_ambiguous_root_without_queue_writes(database):
    sources = AdmittedSources()
    monitor = active_monitor(database, sources, "歧义根帖")
    parent = search_run(database, sources, monitor, "ambiguous-comment-tracking-search")
    match_id, _ = seed_root(
        database,
        parent.id,
        parent.monitor_version_id,
        "bvid:BV1BVFWeHEaV",
    )
    now = utcnow()
    with database.begin() as session:
        session.add(
            Content(
                id=uuid4(),
                source="bilibili",
                provider_namespace="article",
                external_id="bvid:BV1BVFWeHEaV",
                kind="post",
                canonical_url=None,
                author_ref="author:ambiguous",
                root_external_id="bvid:BV1BVFWeHEaV",
                parent_external_id=None,
                relation_status="root",
                visibility="available",
                first_seen_at=now,
                last_seen_at=now,
            )
        )

    with pytest.raises(AppError, match="comment_root_unresolved"):
        CollectionService(database, sources, evidence_configured=True).start_comment_tracking(
            match_id
        )
    with database() as session:
        assert session.get(MonitorMatch, match_id).review_state == "new"
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 1
        assert session.scalar(select(func.count()).select_from(Job)) == 1
        assert session.scalar(select(func.count()).select_from(Outbox)) == 1
