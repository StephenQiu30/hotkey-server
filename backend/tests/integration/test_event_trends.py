from datetime import UTC, datetime, timedelta
from hashlib import sha256
from uuid import UUID, uuid4

import pytest

from collection.models import CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from contents.models import Content, ContentObservation, ContentVersion
from events.schemas import EventInput, EventMemberInput
from events.services import EventService
from evidence.models import RawPage
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


def active_monitor(database):
    sources = AdmittedSources()
    service = MonitorService(database, sources)
    created = service.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "趋势测试",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "budget": {"daily_requests": 100, "content_purchase_cost": 0},
            }
        )
    )
    return sources, service.change_state(
        created.id,
        MonitorStateChange(expected_version=created.current_version),
        "active",
    )


def collection_run(
    database,
    service: CollectionService,
    monitor,
    *,
    key: str,
    mode: str,
    completed_at: datetime,
    policy: str = "policy-v1",
    state: str = "completed",
    outcome: str | None = "ok",
) -> UUID:
    window_since = completed_at.replace(hour=0, minute=0, second=0, microsecond=0)
    created = service.create_run(
        CollectionRunInput(
            monitor_id=monitor.id,
            expected_version=monitor.current_version,
            source="bilibili",
            request_value="AI",
            since=window_since,
            until=window_since + timedelta(days=1),
            idempotency_key=key,
            policy_version=policy,
            retention_days=7,
            ingestion_mode=mode,
        )
    )
    with database.begin() as session:
        run = session.get(CollectionRun, created.id)
        assert run is not None
        run.state = state
        run.outcome = outcome
        run.completed_at = completed_at if state in {"completed", "failed", "cancelled"} else None
    return created.id


def raw_page(database, run_id: UUID, observed_at: datetime, suffix: str) -> UUID:
    identity = uuid4()
    digest = sha256(suffix.encode()).hexdigest()
    with database.begin() as session:
        session.add(
            RawPage(
                id=identity,
                run_id=run_id,
                source="bilibili",
                operation="search_posts",
                request_fingerprint=digest,
                bucket="synthetic",
                object_key=f"raw/synthetic/{suffix}.json.gz",
                payload_sha256=digest,
                object_sha256=digest,
                response_bytes=1,
                object_bytes=1,
                media_type="application/json",
                observed_at=observed_at,
                retention_until=observed_at + timedelta(days=7),
                policy_version="policy-v1",
            )
        )
    return identity


def content(
    database,
    *,
    raw_page_id: UUID,
    observed_at: datetime,
    suffix: str,
    kind: str,
    reply_count: int | None,
) -> UUID:
    identity = uuid4()
    text_hash = sha256(suffix.encode()).hexdigest()
    with database.begin() as session:
        session.add(
            Content(
                id=identity,
                source="bilibili",
                provider_namespace="video",
                external_id=f"synthetic:{suffix}",
                kind=kind,
                canonical_url=None,
                author_ref="synthetic-author",
                root_external_id=f"synthetic:{suffix}",
                parent_external_id=None,
                relation_status="root" if kind == "post" else "unresolved",
                visibility="available",
                first_seen_at=observed_at,
                last_seen_at=observed_at,
            )
        )
        session.add(
            ContentVersion(
                id=uuid4(),
                content_id=identity,
                version=1,
                text=suffix,
                text_sha256=text_hash,
                published_at=observed_at - timedelta(hours=1),
                observed_at=observed_at,
                raw_page_id=raw_page_id,
            )
        )
        session.add(
            ContentObservation(
                id=uuid4(),
                content_id=identity,
                observed_at=observed_at,
                reply_count=reply_count,
                raw_page_id=raw_page_id,
            )
        )
    return identity


def observation(
    database,
    content_id: UUID,
    raw_page_id: UUID,
    observed_at: datetime,
    reply_count: int,
) -> None:
    with database.begin() as session:
        content_row = session.get(Content, content_id)
        assert content_row is not None
        content_row.last_seen_at = max(content_row.last_seen_at, observed_at)
        session.add(
            ContentObservation(
                id=uuid4(),
                content_id=content_id,
                observed_at=observed_at,
                reply_count=reply_count,
                raw_page_id=raw_page_id,
            )
        )


def test_trend_counts_live_evidence_and_excludes_backfill(database):
    start = datetime(2026, 9, 10, tzinfo=UTC)
    sources, monitor = active_monitor(database)
    collection = CollectionService(database, sources, evidence_configured=True)
    first_live = collection_run(
        database,
        collection,
        monitor,
        key="trend-live-1",
        mode="live",
        completed_at=start + timedelta(hours=2),
    )
    second_live = collection_run(
        database,
        collection,
        monitor,
        key="trend-live-2",
        mode="live",
        completed_at=start + timedelta(days=1, hours=2),
    )
    backfill = collection_run(
        database,
        collection,
        monitor,
        key="trend-backfill",
        mode="backfill",
        completed_at=start + timedelta(hours=3),
    )
    live_page = raw_page(database, first_live, start + timedelta(hours=4), "live-first")
    second_live_page = raw_page(
        database, second_live, start + timedelta(days=1, hours=4), "live-second"
    )
    backfill_page = raw_page(database, backfill, start + timedelta(days=1, hours=3), "backfill")
    post_id = content(
        database,
        raw_page_id=live_page,
        observed_at=start + timedelta(hours=4),
        suffix="live-post",
        kind="post",
        reply_count=3,
    )
    historical_comment_id = content(
        database,
        raw_page_id=backfill_page,
        observed_at=start + timedelta(hours=5),
        suffix="historical-comment",
        kind="comment",
        reply_count=2,
    )
    observation(database, post_id, backfill_page, start + timedelta(days=1, hours=3), 99)
    observation(database, post_id, second_live_page, start + timedelta(days=1, hours=4), 8)

    events = EventService(database)
    event = events.create(EventInput(title="AI发布"))
    events.add_member(event.id, EventMemberInput(content_id=post_id))
    events.add_member(event.id, EventMemberInput(content_id=historical_comment_id))

    trend = events.trends(event.id, start, start + timedelta(days=2), 24)
    assert trend.metric_version == "event-trend-v1"
    assert len(trend.sources) == 1
    first, second = trend.sources[0].buckets
    assert (first.new_posts, first.new_discussions, first.observed_reply_delta) == (1, 0, 0)
    assert first.excluded_backfill_items == 1
    assert first.coverage_status == "comparable" and first.backfill_run_count == 1
    assert (second.new_posts, second.new_discussions, second.observed_reply_delta) == (0, 0, 5)
    assert second.excluded_backfill_observations == 1
    assert second.coverage_status == "comparable"


def test_trend_marks_failures_policy_changes_and_missing_coverage(database):
    start = datetime(2026, 9, 10, tzinfo=UTC)
    sources, monitor = active_monitor(database)
    collection = CollectionService(database, sources, evidence_configured=True)
    first = collection_run(
        database,
        collection,
        monitor,
        key="coverage-v1",
        mode="live",
        completed_at=start + timedelta(hours=2),
    )
    collection_run(
        database,
        collection,
        monitor,
        key="coverage-v2-failed",
        mode="live",
        completed_at=start + timedelta(days=1, hours=2),
        policy="policy-v2",
        state="failed",
        outcome="failed",
    )
    first_page = raw_page(database, first, start + timedelta(hours=3), "coverage-first")
    content_id = content(
        database,
        raw_page_id=first_page,
        observed_at=start + timedelta(hours=3),
        suffix="coverage-post",
        kind="post",
        reply_count=0,
    )
    events = EventService(database)
    event = events.create(EventInput(title="覆盖测试"))
    events.add_member(event.id, EventMemberInput(content_id=content_id))

    buckets = events.trends(event.id, start, start + timedelta(days=3), 24).sources[0].buckets
    assert buckets[0].coverage_status == "interrupted"
    assert buckets[0].interruption_reasons == ["policy_changed"]
    assert buckets[1].coverage_status == "interrupted"
    assert buckets[1].interruption_reasons == ["policy_changed", "run_failed"]
    assert buckets[2].coverage_status == "missing"
    assert buckets[2].interruption_reasons == ["no_live_coverage"]
