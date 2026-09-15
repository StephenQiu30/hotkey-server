import os
from datetime import UTC, datetime, timedelta
from hashlib import sha256
from uuid import UUID, uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import func, select

from ai.contracts import EmbeddingBatch, EmbeddingProviderError
from analysis.schemas import AnalysisLabelInput, AnalysisRunInput
from analysis.services import AnalysisService
from collection.models import CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from contents.models import Content, ContentVersion
from core.config import Settings
from core.errors import AppError
from events.schemas import EventInput, EventMemberInput
from events.services import EventService
from evidence.models import RawPage
from identity.services import IdentityService
from knowledge.models import KnowledgeChunk, KnowledgeCitation, KnowledgeEntry, KnowledgeVersion
from knowledge.services import KnowledgeService
from main import create_app
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


class StaticEmbeddingProvider:
    def embed(self, texts: list[str]) -> EmbeddingBatch:
        vector = [1.0, *([0.0] * 1023)]
        return EmbeddingBatch(
            model="qwen3-embedding:latest",
            model_digest=("64b933495768fbd3b87c20583d379728a07471e0c66733a9df87cd1901b3c44b"),
            dimensions=1024,
            vectors=[vector for _ in texts],
        )


class FailingEmbeddingProvider:
    def embed(self, texts: list[str]) -> EmbeddingBatch:
        raise EmbeddingProviderError("embedding_unavailable")


def _raw_page(database, start: datetime) -> UUID:
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    draft = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "知识检索测试",
                "query_spec": {"include_any": ["热点"]},
                "source_ids": ["bilibili"],
                "budget": {"daily_requests": 100, "content_purchase_cost": 0},
            }
        )
    )
    monitor = monitors.change_state(
        draft.id,
        MonitorStateChange(expected_version=draft.current_version),
        "active",
    )
    run_view = CollectionService(database, sources, evidence_configured=True).create_run(
        CollectionRunInput(
            monitor_id=monitor.id,
            expected_version=monitor.current_version,
            source="bilibili",
            request_value="热点",
            since=start,
            until=start + timedelta(days=3),
            idempotency_key="knowledge-bilibili",
            policy_version="knowledge-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    observed_at = start + timedelta(hours=1)
    digest = sha256(b"knowledge-page").hexdigest()
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
                source="bilibili",
                operation="list_comments",
                request_fingerprint=digest,
                bucket="synthetic",
                object_key="raw/synthetic/knowledge-bilibili.json.gz",
                payload_sha256=digest,
                object_sha256=digest,
                response_bytes=1,
                object_bytes=1,
                media_type="application/json",
                observed_at=observed_at,
                retention_until=observed_at + timedelta(days=7),
                policy_version="knowledge-policy-v1",
            )
        )
    return raw_page_id


def _content(
    database,
    raw_page_id: UUID,
    *,
    external_id: str,
    kind: str,
    root_external_id: str,
    text: str,
    published_at: datetime,
) -> UUID:
    content_id = uuid4()
    with database.begin() as session:
        session.add(
            Content(
                id=content_id,
                source="bilibili",
                provider_namespace="video" if kind == "post" else "comment",
                external_id=external_id,
                kind=kind,
                canonical_url=f"https://example.test/bilibili/{external_id}",
                author_ref="synthetic-author",
                root_external_id=root_external_id,
                parent_external_id=None,
                relation_status="root" if kind == "post" else "resolved",
                visibility="available",
                first_seen_at=published_at,
                last_seen_at=published_at,
            )
        )
        session.add(
            ContentVersion(
                id=uuid4(),
                content_id=content_id,
                version=1,
                text=text,
                text_sha256=sha256(text.encode()).hexdigest(),
                published_at=published_at,
                observed_at=published_at,
                raw_page_id=raw_page_id,
            )
        )
    return content_id


def _completed_analysis(database):
    start = datetime(2026, 9, 10, tzinfo=UTC)
    raw_page_id = _raw_page(database, start)
    root_id = _content(
        database,
        raw_page_id,
        external_id="knowledge-root",
        kind="post",
        root_external_id="knowledge-root",
        text="明确合成的事件根帖",
        published_at=start,
    )
    for position, text in enumerate(("发布功能连续失败", "客服已经公开回应"), start=1):
        _content(
            database,
            raw_page_id,
            external_id=f"knowledge-comment-{position}",
            kind="comment",
            root_external_id="knowledge-root",
            text=text,
            published_at=start + timedelta(days=position),
        )
    events = EventService(database)
    event = events.create(EventInput(title="合成热点事件"))
    event = events.add_member(event.id, EventMemberInput(content_id=root_id))
    analyses = AnalysisService(database)
    run = analyses.create(
        event.id,
        AnalysisRunInput(
            expected_event_revision=event.current_revision,
            since=start,
            until=start + timedelta(days=3),
            cutoff=start + timedelta(days=3),
            max_items=2,
        ),
    )
    first, second = run.samples
    analyses.label(
        run.id,
        first.id,
        AnalysisLabelInput(
            topic="稳定性",
            target="发布功能",
            sentiment="negative",
            stance="oppose",
            request="修复稳定性 %_ literal",
            citation_content_version_ids=[first.contexts[0].content_version_id],
        ),
    )
    completed = analyses.label(
        run.id,
        second.id,
        AnalysisLabelInput(
            topic="官方回应",
            target="客服响应",
            sentiment="neutral",
            stance="neutral",
            request="公开说明",
            citation_content_version_ids=[second.contexts[0].content_version_id],
        ),
    )
    return event, completed, raw_page_id


def test_analysis_snapshot_can_be_published_found_and_marked_stale(database):
    event, analysis, raw_page_id = _completed_analysis(database)
    service = KnowledgeService(database)

    published = service.publish_analysis(analysis.id)
    replayed = service.publish_analysis(analysis.id)

    assert replayed.id == published.id
    assert published.entry_type == "analysis_snapshot"
    assert published.version == 1 and not published.stale
    assert len(published.citations) == 2
    assert all(citation.available for citation in published.citations)
    assert "固定样本 2 条" in published.body
    assert "修复稳定性 %_ literal" in published.body
    assert service.search("修复稳定性", None, 20).items[0].id == published.id
    assert service.search("%_", None, 20).items[0].id == published.id
    assert service.search(None, event.id, 20).items[0].id == published.id
    assert service.search(None, uuid4(), 20).items == []
    with database() as session:
        assert session.scalar(select(func.count()).select_from(KnowledgeEntry)) == 1
        assert session.scalar(select(func.count()).select_from(KnowledgeVersion)) == 1
        assert session.scalar(select(func.count()).select_from(KnowledgeCitation)) == 2
        chunk = session.scalar(select(KnowledgeChunk))
        assert chunk is not None and chunk.index_state == "pending"

    changed = analysis.samples[0].contexts[0]
    changed_text = "修改后的合成评论"
    with database.begin() as session:
        session.add(
            ContentVersion(
                id=uuid4(),
                content_id=changed.content_id,
                version=2,
                text=changed_text,
                text_sha256=sha256(changed_text.encode()).hexdigest(),
                published_at=analysis.since + timedelta(days=1),
                observed_at=analysis.until + timedelta(hours=1),
                raw_page_id=raw_page_id,
            )
        )
    stale = service.get(published.id)
    assert stale.stale
    assert any(not citation.available and citation.text is None for citation in stale.citations)
    assert service.search("修复稳定性", None, 20).items[0].stale


def test_pending_analysis_cannot_be_published(database):
    event, completed, _ = _completed_analysis(database)
    pending = AnalysisService(database).create(
        event.id,
        AnalysisRunInput(
            expected_event_revision=event.current_revision,
            since=completed.since,
            until=completed.until,
            cutoff=completed.cutoff,
            max_items=1,
        ),
    )
    with pytest.raises(AppError) as not_ready:
        KnowledgeService(database).publish_analysis(pending.id)
    assert not_ready.value.code == "analysis_not_ready"


def test_authenticated_http_can_publish_and_search_analysis_snapshot(database):
    _, analysis, _ = _completed_analysis(database)
    IdentityService(database).bootstrap("knowledge-owner", "Test-password-123!")
    settings = Settings(
        database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
        broker_url="amqp://u:p@localhost/test",
        allowed_origins=["http://testserver"],
    )
    with TestClient(create_app(settings)) as client:
        client.headers["Origin"] = "http://testserver"
        login = client.post(
            "/api/v1/session",
            json={"username": "knowledge-owner", "password": "Test-password-123!"},
        )
        assert login.status_code == 200
        client.headers["X-CSRF-Token"] = client.cookies["hk_csrf"]
        path = f"/api/v1/analysis-runs/{analysis.id}/knowledge-entry"
        first = client.post(path)
        replay = client.post(path)
        assert first.status_code == 201
        assert replay.status_code == 201
        assert replay.json()["id"] == first.json()["id"]

        page = client.get("/api/v1/knowledge", params={"query": "修复稳定性"})
        assert page.status_code == 200
        assert page.json()["query_mode"] == "exact_substring"
        assert page.json()["items"][0]["id"] == first.json()["id"]
        detail = client.get(f"/api/v1/knowledge/{first.json()['id']}")
        assert detail.status_code == 200
        assert len(detail.json()["citations"]) == 2
        unavailable = client.post(f"/api/v1/knowledge/{first.json()['id']}/semantic-index")
        assert unavailable.status_code == 503
        assert unavailable.json()["code"] == "embedding_not_configured"
        semantic = client.get(
            "/api/v1/knowledge", params={"query": "release outage", "mode": "semantic"}
        )
        assert semantic.status_code == 503
        assert semantic.json()["code"] == "embedding_not_configured"


def test_semantic_index_failure_can_be_retried_and_filtered_by_event(database):
    event, analysis, _ = _completed_analysis(database)
    published = KnowledgeService(database).publish_analysis(analysis.id)

    with pytest.raises(AppError) as failed:
        KnowledgeService(database, FailingEmbeddingProvider()).index(published.id)
    assert failed.value.code == "embedding_unavailable"
    with database() as session:
        chunk = session.scalar(select(KnowledgeChunk))
        assert chunk is not None and chunk.index_state == "failed"

    service = KnowledgeService(database, StaticEmbeddingProvider())
    indexed = service.index(published.id)
    assert indexed.semantic_index_state == "ready"

    page = service.search("release outage", event.id, 20, "semantic")
    assert page.query_mode == "semantic"
    assert page.items[0].id == published.id
    assert page.items[0].similarity == pytest.approx(1.0)
    assert service.search("release outage", uuid4(), 20, "semantic").items == []
