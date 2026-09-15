from datetime import UTC, datetime, timedelta
from hashlib import sha256
from uuid import UUID, uuid4

import pytest

from analysis.schemas import AnalysisLabelInput, AnalysisRunInput
from analysis.services import AnalysisService
from collection.models import CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from contents.models import Content, ContentVersion
from core.errors import AppError
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


def collection_context(database, start: datetime):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    draft = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "分析样本测试",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili", "bluesky"],
                "budget": {"daily_requests": 1000, "content_purchase_cost": 0},
            }
        )
    )
    monitor = monitors.change_state(
        draft.id,
        MonitorStateChange(expected_version=draft.current_version),
        "active",
    )
    collection = CollectionService(database, sources, evidence_configured=True)
    pages: dict[str, UUID] = {}
    for source in ("bilibili", "bluesky"):
        run_view = collection.create_run(
            CollectionRunInput(
                monitor_id=monitor.id,
                expected_version=monitor.current_version,
                source=source,
                request_value="AI",
                since=start,
                until=start + timedelta(days=3),
                idempotency_key=f"analysis-{source}",
                policy_version="analysis-policy-v1",
                retention_days=7,
                ingestion_mode="live",
            )
        )
        observed_at = start + timedelta(hours=1)
        digest = sha256(source.encode()).hexdigest()
        raw_page_id = uuid4()
        with database.begin() as session:
            run = session.get(CollectionRun, run_view.id)
            assert run is not None
            run.state = "completed"
            run.outcome = "ok"
            run.completed_at = observed_at
            session.add(
                RawPage(
                    id=raw_page_id,
                    run_id=run.id,
                    source=source,
                    operation="list_comments",
                    request_fingerprint=digest,
                    bucket="synthetic",
                    object_key=f"raw/synthetic/analysis-{source}.json.gz",
                    payload_sha256=digest,
                    object_sha256=digest,
                    response_bytes=1,
                    object_bytes=1,
                    media_type="application/json",
                    observed_at=observed_at,
                    retention_until=observed_at + timedelta(days=7),
                    policy_version="analysis-policy-v1",
                )
            )
        pages[source] = raw_page_id
    return pages


def add_content(
    database,
    *,
    source: str,
    raw_page_id: UUID,
    external_id: str,
    kind: str,
    root_external_id: str,
    published_at: datetime,
    observed_at: datetime,
    text: str,
    parent_external_id: str | None = None,
    relation_status: str | None = None,
) -> tuple[UUID, UUID]:
    content_id = uuid4()
    version_id = uuid4()
    with database.begin() as session:
        session.add(
            Content(
                id=content_id,
                source=source,
                provider_namespace="video" if kind == "post" else "comment",
                external_id=external_id,
                kind=kind,
                canonical_url=f"https://example.test/{source}/{external_id}",
                author_ref="synthetic-author",
                root_external_id=root_external_id,
                parent_external_id=parent_external_id,
                relation_status=relation_status or ("root" if kind == "post" else "resolved"),
                visibility="available",
                first_seen_at=observed_at,
                last_seen_at=observed_at,
            )
        )
        session.add(
            ContentVersion(
                id=version_id,
                content_id=content_id,
                version=1,
                text=text,
                text_sha256=sha256(text.encode()).hexdigest(),
                published_at=published_at,
                observed_at=observed_at,
                raw_page_id=raw_page_id,
            )
        )
    return content_id, version_id


def test_frozen_sample_labels_recompute_and_staleness(database):
    start = datetime(2026, 9, 10, tzinfo=UTC)
    cutoff = start + timedelta(days=3)
    pages = collection_context(database, start)
    roots: list[UUID] = []
    comment_versions: dict[str, UUID] = {}
    for source, suffix in (("bilibili", "b"), ("bluesky", "x")):
        root_external_id = f"root-{suffix}"
        root_id, _ = add_content(
            database,
            source=source,
            raw_page_id=pages[source],
            external_id=root_external_id,
            kind="post",
            root_external_id=root_external_id,
            published_at=start,
            observed_at=start + timedelta(hours=1),
            text=f"{source} root context",
        )
        roots.append(root_id)
        for day in (1, 2):
            _, version_id = add_content(
                database,
                source=source,
                raw_page_id=pages[source],
                external_id=f"comment-{suffix}-{day}",
                kind="comment",
                root_external_id=root_external_id,
                published_at=start + timedelta(days=day, hours=2),
                observed_at=start + timedelta(days=day, hours=3),
                text=f"{source} opinion {day}",
            )
            comment_versions[f"{source}-{day}"] = version_id

    events = EventService(database)
    event = events.create(EventInput(title="跨平台发布争议"))
    for root_id in roots:
        event = events.add_member(event.id, EventMemberInput(content_id=root_id))
    data = AnalysisRunInput(
        expected_event_revision=event.current_revision,
        since=start,
        until=cutoff,
        cutoff=cutoff,
        max_items=3,
    )
    service = AnalysisService(database)
    frozen = service.create(event.id, data)
    replayed = service.create(event.id, data)

    assert replayed.id == frozen.id
    assert frozen.sample_count == 3
    assert frozen.composition.platforms == {"bilibili": 2, "bluesky": 1}
    assert frozen.composition.roots == 2
    assert frozen.composition.ordering_origins == {"provider_default": 3}
    assert len(frozen.composition.time_buckets) == 2
    assert all(sample.contexts[0].role == "sample" for sample in frozen.samples)
    assert all(
        any(context.role == "root" for context in sample.contexts) for sample in frozen.samples
    )

    first, second, third = frozen.samples
    with pytest.raises(AppError) as invalid_citation:
        service.label(
            frozen.id,
            first.id,
            AnalysisLabelInput(
                topic="产品体验",
                target="发布功能",
                sentiment="negative",
                stance="oppose",
                citation_content_version_ids=[uuid4()],
            ),
        )
    assert invalid_citation.value.code == "analysis_citation_outside_sample"

    first_label = AnalysisLabelInput(
        topic="产品体验",
        target="发布功能",
        sentiment="negative",
        stance="oppose",
        request="修复稳定性",
        citation_content_version_ids=[first.contexts[0].content_version_id],
    )
    service.label(frozen.id, first.id, first_label)
    service.label(
        frozen.id,
        second.id,
        AnalysisLabelInput(
            sentiment="unknown",
            stance="unknown",
            abstained=True,
        ),
    )
    completed = service.label(
        frozen.id,
        third.id,
        AnalysisLabelInput(
            topic="产品体验",
            target="发布功能",
            sentiment="negative",
            stance="oppose",
            request="修复稳定性",
            citation_content_version_ids=[third.contexts[0].content_version_id],
        ),
    )
    assert service.label(frozen.id, first.id, first_label).id == frozen.id
    assert completed.status == "succeeded"
    assert (completed.sample_count, completed.valid_labeled_count, completed.abstained_count) == (
        3,
        2,
        1,
    )
    assert len(completed.viewpoints) == 1
    assert completed.viewpoints[0].sample_count == 2
    assert len(completed.viewpoints[0].citations) == 2

    before_manifest = completed.manifest_sha256
    before_viewpoints = completed.viewpoints
    add_content(
        database,
        source="bluesky",
        raw_page_id=pages["bluesky"],
        external_id="comment-x-late",
        kind="comment",
        root_external_id="root-x",
        published_at=start + timedelta(days=2, hours=4),
        observed_at=cutoff + timedelta(hours=1),
        text="arrived after cutoff",
    )
    recomputed = service.recompute(frozen.id)
    assert recomputed.recomputed_from_manifest is True
    assert recomputed.manifest_sha256 == before_manifest
    assert recomputed.sample_count == 3
    assert recomputed.viewpoints == before_viewpoints

    sampled = frozen.samples[0].contexts[0]
    with database.begin() as session:
        session.add(
            ContentVersion(
                id=uuid4(),
                content_id=sampled.content_id,
                version=2,
                text="edited opinion",
                text_sha256=sha256(b"edited opinion").hexdigest(),
                published_at=start + timedelta(days=1, hours=2),
                observed_at=cutoff + timedelta(hours=2),
                raw_page_id=pages[frozen.samples[0].source],
            )
        )
    stale = service.get(frozen.id)
    assert stale.status == "stale"
    assert stale.manifest_sha256 == before_manifest
    assert stale.samples[0].contexts[0].text != "edited opinion"


def test_analysis_requires_current_event_revision_and_comment_sample(database):
    start = datetime(2026, 9, 10, tzinfo=UTC)
    service = AnalysisService(database)
    event = EventService(database).create(EventInput(title="空事件"))
    request = AnalysisRunInput(
        expected_event_revision=event.current_revision,
        since=start,
        until=start + timedelta(days=1),
        cutoff=start + timedelta(days=1),
    )
    with pytest.raises(AppError) as empty:
        service.create(event.id, request)
    assert empty.value.code == "analysis_sample_empty"

    stale_request = request.model_copy(update={"expected_event_revision": 2})
    with pytest.raises(AppError) as conflict:
        service.create(event.id, stale_request)
    assert conflict.value.code == "version_conflict"


def test_root_expansion_excludes_unresolved_comments_but_explicit_member_is_allowed(database):
    start = datetime(2026, 9, 10, tzinfo=UTC)
    pages = collection_context(database, start)
    root_id, _ = add_content(
        database,
        source="bilibili",
        raw_page_id=pages["bilibili"],
        external_id="unresolved-root",
        kind="post",
        root_external_id="unresolved-root",
        published_at=start,
        observed_at=start,
        text="root",
    )
    comment_id, _ = add_content(
        database,
        source="bilibili",
        raw_page_id=pages["bilibili"],
        external_id="unresolved-comment",
        kind="comment",
        root_external_id="unresolved-root",
        published_at=start + timedelta(hours=1),
        observed_at=start + timedelta(hours=1),
        text="unresolved relation",
        relation_status="unresolved",
    )
    events = EventService(database)
    event = events.create(EventInput(title="关系未解析事件"))
    event = events.add_member(event.id, EventMemberInput(content_id=root_id))
    data = AnalysisRunInput(
        expected_event_revision=event.current_revision,
        since=start,
        until=start + timedelta(days=1),
        cutoff=start + timedelta(days=1),
    )
    service = AnalysisService(database)
    with pytest.raises(AppError) as unresolved:
        service.create(event.id, data)
    assert unresolved.value.code == "analysis_sample_empty"

    event = events.add_member(event.id, EventMemberInput(content_id=comment_id))
    direct = service.create(
        event.id,
        data.model_copy(update={"expected_event_revision": event.current_revision}),
    )
    assert direct.sample_count == 1
    assert direct.samples[0].contexts[0].content_id == comment_id
