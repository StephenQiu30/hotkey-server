import os
from datetime import UTC, datetime, timedelta
from hashlib import sha256
from uuid import UUID, uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import func, select

from collection.models import CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from contents.models import Content, ContentObservation, ContentVersion
from contents.services import ContentService
from core.config import Settings
from core.errors import AppError
from evidence.models import RawPage
from identity.services import IdentityService
from jobs.models import Job
from main import create_app
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


def seed_discussion(database) -> dict[str, UUID]:
    started_at = datetime(2026, 9, 16, 8, 0, tzinfo=UTC)
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    draft = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "评论上下文测试",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "budget": {"daily_requests": 192, "content_purchase_cost": 0},
            }
        )
    )
    monitor = monitors.change_state(
        draft.id,
        MonitorStateChange(expected_version=draft.current_version),
        "active",
    )
    collection = CollectionService(database, sources, evidence_configured=True)
    run = collection.create_run(
        CollectionRunInput(
            monitor_id=monitor.id,
            expected_version=monitor.current_version,
            source="bilibili",
            request_value="AI",
            since=started_at,
            until=started_at + timedelta(hours=1),
            idempotency_key="content-detail-test",
            policy_version="synthetic-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    digest = sha256(b"content-detail-test").hexdigest()
    raw_page_id = uuid4()
    identities = {
        name: uuid4()
        for name in (
            "root",
            "comment",
            "reply",
            "ambiguous_a",
            "ambiguous_b",
            "unresolved",
            "withdrawn",
        )
    }
    rows = [
        ("root", "video", "BV1DETAIL", "post", "BV1DETAIL", None, "root", "根帖正文", 12),
        (
            "comment",
            "comment",
            "comment:1",
            "comment",
            "BV1DETAIL",
            None,
            "resolved",
            "根评论正文",
            2,
        ),
        (
            "reply",
            "comment",
            "reply:1",
            "reply",
            "BV1DETAIL",
            "comment:1",
            "resolved",
            "回复正文",
            0,
        ),
        (
            "ambiguous_a",
            "video",
            "AMBIGUOUS",
            "post",
            "AMBIGUOUS",
            None,
            "root",
            "歧义根帖A",
            1,
        ),
        (
            "ambiguous_b",
            "article",
            "AMBIGUOUS",
            "post",
            "AMBIGUOUS",
            None,
            "root",
            "歧义根帖B",
            1,
        ),
        (
            "unresolved",
            "comment",
            "comment:ambiguous",
            "comment",
            "AMBIGUOUS",
            None,
            "unresolved",
            "歧义评论",
            0,
        ),
        (
            "withdrawn",
            "comment",
            "comment:withdrawn",
            "comment",
            "BV1DETAIL",
            None,
            "resolved",
            "已撤权评论",
            0,
        ),
    ]
    with database.begin() as session:
        persisted_run = session.get(CollectionRun, run.id)
        assert persisted_run is not None
        persisted_run.state = "completed"
        persisted_run.outcome = "ok"
        persisted_run.completed_at = started_at
        session.add(
            RawPage(
                id=raw_page_id,
                run_id=run.id,
                source="bilibili",
                operation="list_comments",
                request_fingerprint=digest,
                bucket="synthetic",
                object_key="raw/synthetic/content-detail.json.gz",
                payload_sha256=digest,
                object_sha256=digest,
                response_bytes=1,
                object_bytes=1,
                media_type="application/json",
                observed_at=started_at,
                retention_until=started_at + timedelta(days=7),
                policy_version="synthetic-v1",
            )
        )
        session.flush()
        for index, (
            name,
            namespace,
            external_id,
            kind,
            root_external_id,
            parent_external_id,
            relation_status,
            text,
            reply_count,
        ) in enumerate(rows):
            observed_at = started_at + timedelta(minutes=index)
            content_id = identities[name]
            visibility = "unavailable" if name == "withdrawn" else "available"
            session.add(
                Content(
                    id=content_id,
                    source="bilibili",
                    provider_namespace=namespace,
                    external_id=external_id,
                    kind=kind,
                    canonical_url=f"https://example.invalid/{external_id}",
                    author_ref=f"author:{name}",
                    root_external_id=root_external_id,
                    parent_external_id=parent_external_id,
                    relation_status=relation_status,
                    visibility=visibility,
                    first_seen_at=observed_at,
                    last_seen_at=observed_at,
                )
            )
            session.add(
                ContentVersion(
                    id=uuid4(),
                    content_id=content_id,
                    version=1,
                    text=text,
                    text_sha256=sha256(text.encode()).hexdigest(),
                    published_at=observed_at,
                    observed_at=observed_at,
                    raw_page_id=raw_page_id,
                )
            )
            session.add(
                ContentObservation(
                    id=uuid4(),
                    content_id=content_id,
                    observed_at=observed_at,
                    reply_count=reply_count,
                    raw_page_id=raw_page_id,
                )
            )
    return identities


def test_content_detail_resolves_context_and_pages_without_side_effects(database):
    identities = seed_discussion(database)
    with database() as session:
        jobs_before = session.scalar(select(func.count()).select_from(Job))
        runs_before = session.scalar(select(func.count()).select_from(CollectionRun))

    service = ContentService(database)
    first = service.detail(identities["reply"], limit=1, cursor=None)

    assert first.selected.id == identities["reply"]
    assert first.root is not None and first.root.id == identities["root"]
    assert first.parent is not None and first.parent.id == identities["comment"]
    assert [item.id for item in first.discussion] == [identities["comment"]]
    assert first.next_cursor is not None

    second = service.detail(identities["reply"], limit=1, cursor=first.next_cursor)
    assert [item.id for item in second.discussion] == [identities["reply"]]
    assert second.next_cursor is None

    unresolved = service.detail(identities["unresolved"], limit=20, cursor=None)
    assert unresolved.root is None
    assert unresolved.parent is None
    assert unresolved.discussion == []

    with pytest.raises(AppError) as missing:
        service.detail(identities["withdrawn"], limit=20, cursor=None)
    assert missing.value.code == "content_not_found"
    assert missing.value.status == 404

    with database() as session:
        assert session.scalar(select(func.count()).select_from(Job)) == jobs_before
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == runs_before


def test_content_detail_rejects_unknown_content_and_invalid_cursor(database):
    service = ContentService(database)
    with pytest.raises(AppError) as missing:
        service.detail(uuid4(), limit=20, cursor=None)
    assert missing.value.code == "content_not_found"
    with pytest.raises(AppError) as invalid_cursor:
        service.detail(uuid4(), limit=20, cursor="bad")
    assert invalid_cursor.value.code == "invalid_cursor"


def test_content_detail_http_contract_uses_authenticated_read(database):
    identities = seed_discussion(database)
    IdentityService(database).bootstrap("learner", "Test-password-123!")
    settings = Settings(
        database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
        broker_url="amqp://u:p@localhost/test",
        allowed_origins=["http://testserver"],
    )
    with TestClient(create_app(settings)) as client:
        assert client.get(f"/api/contents/{identities['reply']}").status_code == 401
        login = client.post(
            "/api/session",
            headers={"Origin": "http://testserver"},
            json={"username": "learner", "password": "Test-password-123!"},
        )
        assert login.status_code == 200
        result = client.get(f"/api/contents/{identities['reply']}?limit=1")
        assert result.status_code == 200
        payload = result.json()
        assert payload["selected"]["id"] == str(identities["reply"])
        assert payload["root"]["id"] == str(identities["root"])
        assert payload["parent"]["id"] == str(identities["comment"])
        assert payload["discussion"][0]["id"] == str(identities["comment"])
        assert payload["next_cursor"] is not None
