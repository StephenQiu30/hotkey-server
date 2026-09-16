from datetime import UTC, datetime, timedelta
from hashlib import sha256
from uuid import UUID, uuid4

from sqlalchemy import delete, select

from collection.models import CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from contents.models import Content, ContentVersion, ContentWithdrawalRecord
from contents.schemas import (
    ContentWithdrawalInput,
    ContentWithdrawalManifest,
    ContentWithdrawalManifestEntry,
)
from evidence.contracts import StoredObject
from evidence.models import RawPage
from evidence.services import EvidenceDeletionService
from knowledge.services import KnowledgeService
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.services import SourceService


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


class MemoryEvidenceStore:
    bucket = "synthetic-evidence"

    def __init__(self, *, fail_deletes: int = 0):
        self.objects: dict[str, bytes] = {}
        self.fail_deletes = fail_deletes

    def put(self, key: str, payload: bytes, expected_sha256: str) -> StoredObject:
        self.objects[key] = payload
        return StoredObject(self.bucket, key, expected_sha256, len(payload))

    def delete(self, key: str, expected_sha256: str) -> None:
        if self.fail_deletes:
            self.fail_deletes -= 1
            raise RuntimeError("synthetic_delete_failure")
        payload = self.objects.get(key)
        if payload is not None and sha256(payload).hexdigest() != expected_sha256:
            raise RuntimeError("evidence_object_conflict")
        self.objects.pop(key, None)


def _shared_page(database) -> tuple[UUID, UUID, UUID, bytes, str]:
    start = datetime(2026, 9, 16, tzinfo=UTC)
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    draft = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "删除生命周期测试",
                "query_spec": {"include_any": ["热点"]},
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
    run_view = CollectionService(database, sources, evidence_configured=True).create_run(
        CollectionRunInput(
            monitor_id=monitor.id,
            expected_version=monitor.current_version,
            source="bilibili",
            request_value="热点",
            since=start,
            until=start + timedelta(days=1),
            idempotency_key=f"deletion-{uuid4()}",
            policy_version="deletion-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    payload = b'{"synthetic":true,"items":["delete","keep"]}'
    object_hash = sha256(payload).hexdigest()
    raw_page_id = uuid4()
    deleted_content_id = uuid4()
    kept_content_id = uuid4()
    with database.begin() as session:
        run = session.get(CollectionRun, run_view.id)
        assert run is not None
        run.state = "completed"
        run.outcome = "ok"
        run.completed_at = start
        session.add(
            RawPage(
                id=raw_page_id,
                run_id=run.id,
                source="bilibili",
                operation="list_comments",
                request_fingerprint=sha256(b"shared-page").hexdigest(),
                bucket="synthetic-evidence",
                object_key="raw/synthetic/shared-page.json.gz",
                payload_sha256=object_hash,
                object_sha256=object_hash,
                response_bytes=len(payload),
                object_bytes=len(payload),
                media_type="application/json",
                observed_at=start,
                retention_until=start + timedelta(days=7),
                policy_version="deletion-policy-v1",
            )
        )
        session.flush()
        for content_id, external_id, text in (
            (deleted_content_id, "delete-me", "应被物理清理的合成正文"),
            (kept_content_id, "keep-me", "共享页中的另一条合成正文"),
        ):
            session.add(
                Content(
                    id=content_id,
                    source="bilibili",
                    provider_namespace="comment",
                    external_id=external_id,
                    kind="comment",
                    canonical_url=f"https://example.test/{external_id}",
                    author_ref=f"author-{external_id}",
                    root_external_id="synthetic-root",
                    parent_external_id=None,
                    relation_status="resolved",
                    visibility="available",
                    first_seen_at=start,
                    last_seen_at=start,
                )
            )
            session.add(
                ContentVersion(
                    id=uuid4(),
                    content_id=content_id,
                    version=1,
                    text=text,
                    text_sha256=sha256(text.encode()).hexdigest(),
                    published_at=start,
                    observed_at=start,
                    raw_page_id=raw_page_id,
                )
            )
    return deleted_content_id, kept_content_id, raw_page_id, payload, object_hash


def test_deleted_content_schedules_whole_shared_page_and_reconciles_idempotently(database):
    deleted_id, kept_id, raw_page_id, payload, _ = _shared_page(database)
    knowledge = KnowledgeService(database)
    withdrawn = knowledge.withdraw_content(deleted_id, ContentWithdrawalInput(reason="deleted"))
    replayed = knowledge.withdraw_content(deleted_id, ContentWithdrawalInput(reason="deleted"))
    assert withdrawn == replayed

    with database() as session:
        raw_page = session.get(RawPage, raw_page_id)
        deleted = session.get(Content, deleted_id)
        kept = session.get(Content, kept_id)
        records = list(session.scalars(select(ContentWithdrawalRecord)))
        assert raw_page is not None and raw_page.object_state == "delete_pending"
        assert deleted is not None and deleted.visibility == "deleted"
        assert kept is not None and kept.visibility == "available"
        assert len(records) == 1 and records[0].visibility == "deleted"

    store = MemoryEvidenceStore()
    store.objects["raw/synthetic/shared-page.json.gz"] = payload
    result = EvidenceDeletionService(database, store).reconcile(limit=10)
    replay = EvidenceDeletionService(database, store).reconcile(limit=10)
    assert result.deleted == 1 and result.failed == 0
    assert replay.deleted == 0 and replay.failed == 0
    assert store.objects == {}

    with database() as session:
        raw_page = session.get(RawPage, raw_page_id)
        deleted = session.get(Content, deleted_id)
        kept = session.get(Content, kept_id)
        deleted_version = session.scalar(
            select(ContentVersion).where(ContentVersion.content_id == deleted_id)
        )
        kept_version = session.scalar(
            select(ContentVersion).where(ContentVersion.content_id == kept_id)
        )
        assert raw_page is not None and raw_page.object_state == "deleted"
        assert raw_page.deleted_at is not None
        assert deleted is not None and deleted.author_ref == ""
        assert deleted_version is not None and deleted_version.text.startswith("[deleted:")
        assert kept is not None and kept.visibility == "available"
        assert kept_version is not None and kept_version.text == "共享页中的另一条合成正文"


def test_purpose_revocation_keeps_evidence_and_delete_failure_can_retry(database):
    deleted_id, _, raw_page_id, payload, _ = _shared_page(database)
    knowledge = KnowledgeService(database)
    knowledge.withdraw_content(deleted_id, ContentWithdrawalInput(reason="purpose_revoked"))
    with database() as session:
        raw_page = session.get(RawPage, raw_page_id)
        assert raw_page is not None and raw_page.object_state == "available"

    knowledge.withdraw_content(deleted_id, ContentWithdrawalInput(reason="deleted"))
    store = MemoryEvidenceStore(fail_deletes=1)
    store.objects["raw/synthetic/shared-page.json.gz"] = payload
    failed = EvidenceDeletionService(database, store).reconcile(limit=1)
    assert failed.failed == 1 and failed.deleted == 0
    with database() as session:
        raw_page = session.get(RawPage, raw_page_id)
        assert raw_page is not None and raw_page.object_state == "failed"
        assert raw_page.cleanup_attempts == 1
        assert raw_page.cleanup_error_code == "evidence_delete_failed"

    recovered = EvidenceDeletionService(database, store).reconcile(limit=1)
    assert recovered.deleted == 1 and recovered.failed == 0


def test_manifest_replay_restores_barrier_and_pending_evidence_cleanup(database):
    deleted_id, _, raw_page_id, _, _ = _shared_page(database)
    knowledge = KnowledgeService(database)
    knowledge.withdraw_content(deleted_id, ContentWithdrawalInput(reason="deleted"))
    manifest = knowledge.export_withdrawal_manifest()
    assert manifest.schema_version == "content-withdrawal-manifest-v1"
    assert len(manifest.entries) == 1

    original_text = "应被物理清理的合成正文"
    with database.begin() as session:
        session.execute(delete(ContentWithdrawalRecord))
        content = session.get(Content, deleted_id)
        raw_page = session.get(RawPage, raw_page_id)
        version = session.scalar(
            select(ContentVersion).where(ContentVersion.content_id == deleted_id)
        )
        assert content is not None and raw_page is not None and version is not None
        content.visibility = "available"
        content.author_ref = "author-delete-me"
        raw_page.object_state = "available"
        raw_page.cleanup_attempts = 0
        raw_page.cleanup_error_code = None
        raw_page.deleted_at = None
        version.text = original_text
        version.text_sha256 = sha256(original_text.encode()).hexdigest()

    first = knowledge.replay_withdrawal_manifest(manifest)
    second = knowledge.replay_withdrawal_manifest(manifest)
    assert first.applied == 1 and first.missing == 0
    assert second.applied == 1 and second.missing == 0
    with database() as session:
        raw_page = session.get(RawPage, raw_page_id)
        content = session.get(Content, deleted_id)
        assert raw_page is not None and raw_page.object_state == "delete_pending"
        assert content is not None and content.visibility == "deleted"
        assert len(list(session.scalars(select(ContentWithdrawalRecord)))) == 1


def test_manifest_replay_persists_missing_identity_tombstone(database):
    effective_at = datetime(2026, 9, 16, tzinfo=UTC)
    manifest = ContentWithdrawalManifest(
        schema_version="content-withdrawal-manifest-v1",
        generated_at=effective_at,
        entries=[
            ContentWithdrawalManifestEntry(
                source="bilibili",
                provider_namespace="comment",
                external_id="missing-after-restore",
                visibility="deleted",
                effective_at=effective_at,
            )
        ],
    )

    replay = KnowledgeService(database).replay_withdrawal_manifest(manifest)
    assert replay.applied == 0 and replay.missing == 1
    with database() as session:
        record = session.scalar(select(ContentWithdrawalRecord))
        assert record is not None
        assert record.external_id == "missing-after-restore"
        assert record.visibility == "deleted"
