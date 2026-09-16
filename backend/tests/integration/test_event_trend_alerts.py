from datetime import UTC, datetime, timedelta
from hashlib import sha256
from uuid import UUID, uuid4

import pytest
from sqlalchemy import func, select

from collection.models import CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from contents.models import Content, ContentObservation, ContentVersion
from core.errors import AppError
from events.schemas import EventInput, EventMemberInput
from events.services import EventService
from evidence.models import RawPage
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from notifications.models import Notification, TrendAlertOccurrence
from notifications.schemas import TrendAlertRuleInput, TrendAlertRuleUpdate
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


def active_monitor(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    created = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "趋势提醒测试",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "budget": {"daily_requests": 100, "content_purchase_cost": 0},
            }
        )
    )
    return sources, monitors.change_state(
        created.id,
        MonitorStateChange(expected_version=created.current_version),
        "active",
    )


def collection_run(database, service, monitor, *, key, mode, state="completed", outcome="ok"):
    starts_at = datetime(2026, 9, 15, tzinfo=UTC)
    created = service.create_run(
        CollectionRunInput(
            monitor_id=monitor.id,
            expected_version=monitor.current_version,
            source="bilibili",
            request_value="AI",
            since=starts_at,
            until=starts_at + timedelta(days=1),
            idempotency_key=key,
            policy_version="policy-v1",
            retention_days=7,
            ingestion_mode=mode,
        )
    )
    with database.begin() as session:
        run = session.get(CollectionRun, created.id)
        assert run is not None
        run.state = state
        run.outcome = outcome
        run.completed_at = starts_at + timedelta(hours=23)
    return created.id


def add_content(database, run_id: UUID, *, suffix: str) -> UUID:
    observed_at = datetime(2026, 9, 15, 10, tzinfo=UTC)
    digest = sha256(suffix.encode()).hexdigest()
    raw_page_id = uuid4()
    content_id = uuid4()
    with database.begin() as session:
        session.add(
            RawPage(
                id=raw_page_id,
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
    with database.begin() as session:
        session.add(
            Content(
                id=content_id,
                source="bilibili",
                provider_namespace="video",
                external_id=f"synthetic:{suffix}",
                kind="post",
                canonical_url=None,
                author_ref="synthetic-author",
                root_external_id=f"synthetic:{suffix}",
                parent_external_id=None,
                relation_status="root",
                visibility="available",
                first_seen_at=observed_at,
                last_seen_at=observed_at,
            )
        )
        session.add(
            ContentVersion(
                id=uuid4(),
                content_id=content_id,
                version=1,
                text=suffix,
                text_sha256=digest,
                published_at=observed_at - timedelta(hours=1),
                observed_at=observed_at,
                raw_page_id=raw_page_id,
            )
        )
        session.add(
            ContentObservation(
                id=uuid4(),
                content_id=content_id,
                observed_at=observed_at,
                reply_count=0,
                raw_page_id=raw_page_id,
            )
        )
    return content_id


def event_with_rule(database, content_id: UUID):
    events = EventService(database)
    event = events.create(EventInput(title="AI发布"))
    events.add_member(event.id, EventMemberInput(content_id=content_id))
    rule = events.create_trend_alert_rule(
        event.id,
        TrendAlertRuleInput(
            source="bilibili",
            metric="new_posts",
            bucket_hours=24,
            threshold_count=1,
        ),
    )
    return events, event, rule


def test_comparable_bucket_alert_is_atomic_idempotent_and_schedulable(database):
    sources, monitor = active_monitor(database)
    collection = CollectionService(database, sources, evidence_configured=True)
    live_run = collection_run(database, collection, monitor, key="alert-live", mode="live")
    content_id = add_content(database, live_run, suffix="comparable")
    events, event, rule = event_with_rule(database, content_id)
    now = datetime(2026, 9, 16, 0, 5, tzinfo=UTC)

    assert events.evaluate_due_trend_alerts(now) == 1
    replay = events.evaluate_trend_alerts(event.id, now)
    assert replay.evaluated_rules == 1 and replay.created_notifications == 0
    with database() as session:
        assert session.scalar(select(func.count()).select_from(TrendAlertOccurrence)) == 1
        notification = session.scalar(
            select(Notification).where(Notification.kind == "trend_threshold_reached")
        )
        assert notification is not None
        assert notification.change_id is None and notification.trend_occurrence_id is not None
        assert notification.read_at is None and notification.rule_version == 1

    disabled = events.update_trend_alert_rule(
        event.id,
        rule.id,
        TrendAlertRuleUpdate(expected_version=1, threshold_count=1, enabled=False),
    )
    assert disabled.version == 2 and disabled.enabled is False
    assert events.evaluate_due_trend_alerts(now) == 0
    with pytest.raises(AppError) as stale:
        events.update_trend_alert_rule(
            event.id,
            rule.id,
            TrendAlertRuleUpdate(expected_version=1, threshold_count=2, enabled=True),
        )
    assert (stale.value.code, stale.value.status) == (
        "trend_alert_rule_version_conflict",
        409,
    )


@pytest.mark.parametrize(
    ("mode", "state", "outcome"),
    [("backfill", "completed", "ok"), ("live", "failed", "failed")],
)
def test_backfill_and_interrupted_buckets_never_alert(database, mode, state, outcome):
    sources, monitor = active_monitor(database)
    collection = CollectionService(database, sources, evidence_configured=True)
    run_id = collection_run(
        database,
        collection,
        monitor,
        key=f"alert-{mode}-{state}",
        mode=mode,
        state=state,
        outcome=outcome,
    )
    content_id = add_content(database, run_id, suffix=f"{mode}-{state}")
    events, event, _ = event_with_rule(database, content_id)

    result = events.evaluate_trend_alerts(event.id, datetime(2026, 9, 16, 0, 5, tzinfo=UTC))
    assert result.evaluated_rules == 1 and result.created_notifications == 0
    with database() as session:
        assert session.scalar(select(func.count()).select_from(TrendAlertOccurrence)) == 0
        assert (
            session.scalar(
                select(func.count())
                .select_from(Notification)
                .where(Notification.kind == "trend_threshold_reached")
            )
            == 0
        )
