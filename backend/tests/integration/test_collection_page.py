from datetime import UTC, datetime, timedelta
from hashlib import sha256
from uuid import uuid4

import pytest
from sqlalchemy import func, select, update

from audit.models import Audit
from collection.execution import CollectionExecutor
from collection.models import CollectionBudgetUsage, CollectionCheckpoint, CollectionRun
from collection.schemas import CollectionRunInput, PageCommitInput
from collection.services import CollectionService
from contents.models import Content, ContentObservation, ContentVersion, ContentWithdrawalRecord
from contents.schemas import ContentWithdrawalManifestEntry
from contents.services import ContentService, apply_withdrawal_manifest_entry
from core.clock import utcnow
from core.errors import AppError
from evidence.contracts import StoredObject
from evidence.models import RawPage
from evidence.services import EvidenceDeletionService
from jobs.contracts import Dispatch
from jobs.execution import claim, complete, fail_lease, reconcile, reschedule_lease
from jobs.models import Attempt, Job, JobResult, Outbox
from jobs.services import JobService
from monitors.models import MonitorMatch
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.contracts import FetchedPage
from sources.schemas import SocialObject, SourceReference, SourceResult
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return []


class RevokedSources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return ["source_not_eligible"]


class SearchOnlySources(SourceService):
    def activation_issues(self, source_ids, operation="search_posts"):
        return [] if operation == "search_posts" else ["source_not_eligible"]


class MemoryStore:
    bucket = "synthetic-evidence"

    def __init__(self):
        self.objects = {}

    def put(self, key: str, payload: bytes, expected_sha256: str) -> StoredObject:
        assert sha256(payload).hexdigest() == expected_sha256
        existing = self.objects.setdefault(key, payload)
        if existing != payload:
            raise RuntimeError("object_conflict")
        return StoredObject(
            bucket=self.bucket,
            key=key,
            sha256=expected_sha256,
            size=len(payload),
        )

    def delete(self, key: str, expected_sha256: str) -> None:
        payload = self.objects.get(key)
        if payload is None:
            return
        assert sha256(payload).hexdigest() == expected_sha256
        del self.objects[key]


class FenceChangingStore(MemoryStore):
    def __init__(self, database, run_id):
        super().__init__()
        self.database = database
        self.run_id = run_id

    def put(self, key: str, payload: bytes, expected_sha256: str) -> StoredObject:
        stored = super().put(key, payload, expected_sha256)
        with self.database.begin() as session:
            run = session.get(CollectionRun, self.run_id)
            assert run is not None
            run.fencing_token += 1
        return stored


class WithdrawalDuringPutStore(MemoryStore):
    def __init__(self, database, *, fail_delete: bool = False):
        super().__init__()
        self.database = database
        self.fail_delete = fail_delete

    def put(self, key: str, payload: bytes, expected_sha256: str) -> StoredObject:
        stored = super().put(key, payload, expected_sha256)
        with self.database.begin() as session:
            apply_withdrawal_manifest_entry(
                session,
                ContentWithdrawalManifestEntry(
                    source="bilibili",
                    provider_namespace="video",
                    external_id="BV1BVFWeHEaV",
                    visibility="deleted",
                    effective_at=utcnow(),
                ),
            )
        return stored

    def delete(self, key: str, expected_sha256: str) -> None:
        if self.fail_delete:
            raise RuntimeError("synthetic_delete_failure")
        super().delete(key, expected_sha256)


class CancelDuringPutStore(MemoryStore):
    def __init__(self, cancel, *, fail_delete: bool = False):
        super().__init__()
        self.cancel = cancel
        self.fail_delete = fail_delete

    def put(self, key: str, payload: bytes, expected_sha256: str) -> StoredObject:
        stored = super().put(key, payload, expected_sha256)
        self.cancel()
        return stored

    def delete(self, key: str, expected_sha256: str) -> None:
        if self.fail_delete:
            raise RuntimeError("synthetic_delete_failure")
        super().delete(key, expected_sha256)


class StaticFetcher:
    def __init__(self, page: FetchedPage):
        self.page = page
        self.calls = 0
        self.inputs = []

    def fetch(self, data):
        self.calls += 1
        self.inputs.append(data)
        return self.page


class SequenceFetcher:
    def __init__(self, pages: list[FetchedPage]):
        self.pages = pages
        self.calls = 0

    def fetch(self, data):
        page = self.pages[self.calls]
        self.calls += 1
        return page


class CancelDuringFetch:
    def __init__(self, page: FetchedPage, cancel):
        self.page = page
        self.cancel = cancel
        self.calls = 0

    def fetch(self, data):
        self.calls += 1
        self.cancel()
        return self.page


def monitor(service: MonitorService, title: str, terms: list[str]):
    created = service.create_monitor(
        MonitorInput.model_validate(
            {
                "title": title,
                "query_spec": {"include_any": terms},
                "source_ids": ["bilibili"],
                "budget": {"daily_requests": 1000, "content_purchase_cost": 0},
            }
        )
    )
    return service.change_state(
        created.id, MonitorStateChange(expected_version=created.current_version), "active"
    )


def result(
    text: str,
    reply_count: int | None,
    payload: bytes,
    external_id: str = "BV1BVFWeHEaV",
    provider_namespace: str = "video",
    kind: str = "post",
    root_id: str | None = None,
    parent_id: str | None = None,
    operation: str = "search_posts",
) -> SourceResult:
    return SourceResult(
        source="bilibili",
        adapter_version="synthetic-transaction-poc",
        operation=operation,
        status="ok",
        observed_at=utcnow(),
        items=[
            SocialObject(
                external_id=external_id,
                provider_namespace=provider_namespace,
                kind=kind,
                text=text,
                author_id="author-7",
                created_at=datetime(2026, 9, 15, tzinfo=UTC),
                root_id=root_id or external_id,
                parent_id=parent_id,
                reply_count=reply_count,
                canonical_url="https://www.bilibili.com/video/BV1BVFWeHEaV",
            )
        ],
        response_bytes=len(payload),
        response_sha256=sha256(payload).hexdigest(),
    )


def failed_page(code: str, retry_after_seconds: int | None = None) -> FetchedPage:
    return FetchedPage(
        result=SourceResult(
            source="bilibili",
            adapter_version="synthetic-retry-poc",
            operation="search_posts",
            status="failed",
            code=code,
            observed_at=utcnow(),
            http_status=429 if code == "rate_limited" else 500,
            retry_after_seconds=retry_after_seconds,
        ),
        payload=None,
        media_type="application/json",
        request_fingerprint=sha256(f"failure:{code}".encode()).hexdigest(),
        page_key=f"failure:{code}",
    )


def create_collection_run(
    service: CollectionService,
    monitor_id,
    version: int,
    key: str,
):
    return service.create_run(
        CollectionRunInput(
            monitor_id=monitor_id,
            expected_version=version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key=key,
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )


def run_and_commit(
    service: CollectionService,
    store: MemoryStore,
    monitor_id,
    version,
    key,
    text,
    count,
    external_id: str = "BV1BVFWeHEaV",
    provider_namespace: str = "video",
    kind: str = "post",
    root_id: str | None = None,
    parent_id: str | None = None,
):
    started = service.create_run(
        CollectionRunInput(
            monitor_id=monitor_id,
            expected_version=version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key=key,
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    job_lease = claim(service.factory, Dispatch(job_id=started.job_id, epoch=1))
    assert job_lease is not None
    lease = service.claim_for_job(job_lease)
    assert lease is not None
    payload = ('{"page":"' + key + '"}').encode()
    page = PageCommitInput(
        run_id=started.id,
        fencing_token=lease.fencing_token,
        page_key="first",
        request_fingerprint=sha256(key.encode()).hexdigest(),
        media_type="application/json",
        payload=payload,
        retention_until=utcnow() + timedelta(days=7),
        policy_version="synthetic-policy-v1",
        result=result(
            text,
            count,
            payload,
            external_id,
            provider_namespace,
            kind,
            root_id,
            parent_id,
        ),
    )
    return service.commit_page(page, store, job_lease), page, job_lease


def test_page_commit_is_idempotent_and_preserves_versions_matches_and_zero(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    first = monitor(monitors, "主题一", ["AI"])
    committed, page, job_lease = run_and_commit(
        collection, store, first.id, first.current_version, "run-one", "AI 初版", None
    )
    duplicate = collection.commit_page(page, store, job_lease)
    assert committed.duplicate is False and duplicate.duplicate is True

    second = monitor(monitors, "主题二", ["AI"])
    run_and_commit(collection, store, second.id, second.current_version, "run-two", "AI 初版", None)
    run_and_commit(collection, store, first.id, first.current_version, "run-three", "AI 第二版", 0)

    with database() as session:
        assert session.scalar(select(func.count()).select_from(Content)) == 1
        assert session.scalar(select(func.count()).select_from(ContentVersion)) == 2
        assert session.scalar(select(func.count()).select_from(MonitorMatch)) == 2
        assert session.scalar(select(func.count()).select_from(CollectionCheckpoint)) == 3
        assert session.scalar(select(func.count()).select_from(RawPage)) == 3
        observations = list(
            session.scalars(select(ContentObservation).order_by(ContentObservation.observed_at))
        )
        assert observations[0].reply_count is None
        assert observations[-1].reply_count == 0


def test_collection_run_and_job_are_atomic_and_executor_commits_one_page(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "执行", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="executor-one-page",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    payload = b'{"page":"executor"}'
    fetcher = StaticFetcher(
        FetchedPage(
            result=result("AI 可入库", 0, payload),
            payload=payload,
            media_type="application/json",
            request_fingerprint=sha256(b"executor-request").hexdigest(),
            page_key="search:executor",
        )
    )
    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None and lease.kind == "collect_page"

    assert CollectionExecutor(collection, sources, fetcher, MemoryStore()).execute(lease)

    with database() as session:
        settled_job = session.get(Job, created.job_id)
        assert settled_job is not None and settled_job.status == "succeeded"
    assert not complete(database, lease)

    persisted = collection.run(created.id)
    assert persisted.state == "completed" and persisted.outcome == "ok"
    assert persisted.pages_count == 1 and persisted.items_count == 1
    assert fetcher.calls == 1
    with database() as session:
        assert session.scalar(select(func.count()).select_from(Job)) == 1
        assert session.scalar(select(func.count()).select_from(Outbox)) == 1
        assert session.scalar(select(func.count()).select_from(JobResult)) == 0


def test_user_cancel_during_source_fetch_atomically_stops_run_and_old_lease(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "请求期间取消", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = create_collection_run(
        collection, active.id, active.current_version, "cancel-during-source-fetch"
    )
    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None
    jobs = JobService(database, {"collect_page": collection.cancel_job})
    payload = b'{"page":"cancel-during-fetch"}'
    page = FetchedPage(
        result=result("不得提交的合成正文", 0, payload),
        payload=payload,
        media_type="application/json",
        request_fingerprint=sha256(b"cancel-during-fetch").hexdigest(),
        page_key="search:cancel-during-fetch",
    )
    store = MemoryStore()

    assert not CollectionExecutor(
        collection,
        sources,
        CancelDuringFetch(page, lambda: jobs.cancel_job(created.job_id)),
        store,
    ).execute(lease)

    with database.begin() as session:
        run = session.get(CollectionRun, created.id)
        job = session.get(Job, created.job_id)
        attempt = session.scalar(select(Attempt).where(Attempt.job_id == created.job_id))
        assert run is not None and job is not None and attempt is not None
        assert run.state == "cancelled" and run.outcome == "partial"
        assert run.stop_reason == "user_cancelled" and run.completed_at is not None
        assert job.status == "cancelled" and job.lease_until is None
        assert attempt.outcome == "cancelled" and attempt.ended_at is not None
        assert session.scalar(select(func.count()).select_from(RawPage)) == 0
        assert session.scalar(select(func.count()).select_from(Content)) == 0
        assert session.scalar(select(func.count()).select_from(CollectionCheckpoint)) == 0
        assert not fail_lease(session, lease)
        assert reschedule_lease(session, lease, 5) == "stale"
    assert store.objects == {}
    assert not complete(database, lease)
    assert claim(database, Dispatch(job_id=created.job_id, epoch=1)) is None


def test_user_cancel_during_object_upload_deletes_rejected_object(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "上传期间取消", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = create_collection_run(
        collection, active.id, active.current_version, "cancel-during-object-upload"
    )
    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None
    jobs = JobService(database, {"collect_page": collection.cancel_job})
    payload = b'{"page":"cancel-during-upload"}'
    page = FetchedPage(
        result=result("不得提交的合成正文", 0, payload),
        payload=payload,
        media_type="application/json",
        request_fingerprint=sha256(b"cancel-during-upload").hexdigest(),
        page_key="search:cancel-during-upload",
    )
    store = CancelDuringPutStore(lambda: jobs.cancel_job(created.job_id))

    assert not CollectionExecutor(collection, sources, StaticFetcher(page), store).execute(lease)
    assert store.objects == {}
    with database() as session:
        run = session.get(CollectionRun, created.id)
        job = session.get(Job, created.job_id)
        assert run is not None and job is not None
        assert run.state == "cancelled" and job.status == "cancelled"
        assert session.scalar(select(func.count()).select_from(RawPage)) == 0
        assert session.scalar(select(func.count()).select_from(Content)) == 0
        assert session.scalar(select(func.count()).select_from(CollectionCheckpoint)) == 0


def test_cancelled_upload_cleanup_failure_is_recorded_and_reconciled(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "取消补偿", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = create_collection_run(
        collection, active.id, active.current_version, "cancelled-upload-cleanup"
    )
    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None
    jobs = JobService(database, {"collect_page": collection.cancel_job})
    payload = b'{"page":"cancelled-upload-cleanup"}'
    page = FetchedPage(
        result=result("不得提交的合成正文", 0, payload),
        payload=payload,
        media_type="application/json",
        request_fingerprint=sha256(b"cancelled-upload-cleanup").hexdigest(),
        page_key="search:cancelled-upload-cleanup",
    )
    store = CancelDuringPutStore(lambda: jobs.cancel_job(created.job_id), fail_delete=True)

    with pytest.raises(AppError, match="evidence_rejection_cleanup_failed"):
        CollectionExecutor(collection, sources, StaticFetcher(page), store).execute(lease)

    with database() as session:
        raw_page = session.scalar(select(RawPage))
        assert raw_page is not None
        assert raw_page.run_id == created.id
        assert raw_page.object_state == "failed" and raw_page.cleanup_attempts == 1
        assert session.scalar(select(func.count()).select_from(Content)) == 0
        assert session.scalar(select(func.count()).select_from(CollectionCheckpoint)) == 0

    store.fail_delete = False
    assert EvidenceDeletionService(database, store).reconcile(1).deleted == 1
    assert store.objects == {}


def test_search_reference_commit_atomically_creates_one_budgeted_detail_run(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "引用展开", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="reference-expansion",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    payload = b'{"references":["BV1BVFWeHEaV"]}'
    page = FetchedPage(
        result=SourceResult(
            source="bilibili",
            adapter_version="synthetic-reference-poc",
            operation="search_posts",
            status="ok",
            observed_at=utcnow(),
            references=[
                SourceReference(
                    external_id="bvid:BV1BVFWeHEaV",
                    canonical_url="https://www.bilibili.com/video/BV1BVFWeHEaV",
                )
            ],
            response_bytes=len(payload),
            response_sha256=sha256(payload).hexdigest(),
        ),
        payload=payload,
        media_type="application/json",
        request_fingerprint=sha256(b"reference-expansion-request").hexdigest(),
        page_key="search:reference-expansion",
    )
    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None
    store = MemoryStore()
    assert CollectionExecutor(collection, sources, StaticFetcher(page), store).execute(lease)
    duplicate = collection.commit_page(
        PageCommitInput(
            run_id=created.id,
            fencing_token=lease.fencing_token,
            page_key=page.page_key,
            request_fingerprint=page.request_fingerprint,
            media_type=page.media_type,
            payload=payload,
            retention_until=page.result.observed_at + timedelta(days=7),
            policy_version="synthetic-policy-v1",
            result=page.result,
        ),
        store,
        lease,
    )
    assert duplicate.duplicate is True
    assert duplicate.followup_run_count == 0

    with database() as session:
        runs = list(session.scalars(select(CollectionRun).order_by(CollectionRun.created_at)))
        assert len(runs) == 2
        detail = runs[1]
        assert detail.parent_run_id == created.id
        assert detail.operation == "fetch_post"
        assert detail.ingestion_mode == "live"
        assert detail.request_value == "bvid:BV1BVFWeHEaV"
        assert session.scalar(select(func.count()).select_from(Job)) == 2
        assert session.scalar(select(func.count()).select_from(Outbox)) == 2
        detail_job_id = detail.job_id
    assert not complete(database, lease)

    detail_payload = b'{"post":"video:113"}'
    detail_page = FetchedPage(
        result=result(
            "AI 详情正文",
            2,
            detail_payload,
            external_id="video:113",
            root_id="video:113",
            operation="fetch_post",
        ),
        payload=detail_payload,
        media_type="application/json",
        request_fingerprint=sha256(b"detail-request").hexdigest(),
        page_key="post:detail",
    )
    detail_lease = claim(database, Dispatch(job_id=detail_job_id, epoch=1))
    assert detail_lease is not None
    assert CollectionExecutor(
        collection, sources, StaticFetcher(detail_page), MemoryStore()
    ).execute(detail_lease)
    assert not complete(database, detail_lease)

    inbox = ContentService(database).inbox(20, None)
    assert len(inbox.items) == 1
    assert inbox.items[0].text == "AI 详情正文"
    assert inbox.items[0].external_id == "video:113"
    with database() as session:
        assert session.scalar(select(func.count()).select_from(RawPage)) == 2
        assert session.scalar(select(func.count()).select_from(CollectionCheckpoint)) == 2


def test_same_scheduled_detail_can_be_expanded_from_distinct_search_parents(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "同帖多关键词", ["AI", "人工智能"])
    collection = CollectionService(database, sources, evidence_configured=True)
    slot = datetime(2026, 9, 15, tzinfo=UTC)
    parents = []
    for index, term in enumerate(("AI", "人工智能"), start=1):
        parent = collection.create_run(
            CollectionRunInput(
                monitor_id=active.id,
                expected_version=active.current_version,
                source="bilibili",
                request_value=term,
                since="2026-09-14T00:00:00Z",
                until="2026-09-15T00:00:00Z",
                idempotency_key=f"same-detail-parent-{index}",
                policy_version="synthetic-policy-v1",
                retention_days=7,
                ingestion_mode="live",
                trigger="scheduled",
                schedule_slot=slot,
            )
        )
        job_lease = claim(database, Dispatch(job_id=parent.job_id, epoch=1))
        assert job_lease is not None
        execution = collection.claim_for_job(job_lease)
        assert execution is not None
        payload = f'{{"search":{index}}}'.encode()
        committed = collection.commit_page(
            PageCommitInput(
                run_id=parent.id,
                fencing_token=execution.fencing_token,
                page_key=f"search:same-detail-{index}",
                request_fingerprint=sha256(f"same-detail-{index}".encode()).hexdigest(),
                media_type="application/json",
                payload=payload,
                retention_until=utcnow() + timedelta(days=7),
                policy_version="synthetic-policy-v1",
                result=SourceResult(
                    source="bilibili",
                    adapter_version="synthetic-reference-poc",
                    operation="search_posts",
                    status="ok",
                    observed_at=utcnow(),
                    references=[
                        SourceReference(
                            external_id="bvid:BV1BVFWeHEaV",
                            canonical_url="https://www.bilibili.com/video/BV1BVFWeHEaV",
                        )
                    ],
                    response_bytes=len(payload),
                    response_sha256=sha256(payload).hexdigest(),
                ),
            ),
            MemoryStore(),
            job_lease,
        )
        assert committed.followup_run_count == 1
        parents.append(parent.id)

    with database() as session:
        children = list(
            session.scalars(select(CollectionRun).where(CollectionRun.operation == "fetch_post"))
        )
        assert len(children) == 2
        assert {child.parent_run_id for child in children} == set(parents)
        assert {child.request_value for child in children} == {"bvid:BV1BVFWeHEaV"}
        assert session.scalar(select(func.count()).select_from(Job)) == 4
        assert session.scalar(select(func.count()).select_from(Outbox)) == 4


def test_post_detail_expands_two_root_comments_with_independent_reply_pages(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "评论上下文", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    search = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="root-comment-search",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    search_payload = b'{"reference":"BV1BVFWeHEaV"}'
    search_page = FetchedPage(
        result=SourceResult(
            source="bilibili",
            adapter_version="synthetic-root-comment-poc",
            operation="search_posts",
            status="ok",
            observed_at=utcnow(),
            references=[
                SourceReference(
                    external_id="bvid:BV1BVFWeHEaV",
                    canonical_url="https://www.bilibili.com/video/BV1BVFWeHEaV",
                )
            ],
            response_bytes=len(search_payload),
            response_sha256=sha256(search_payload).hexdigest(),
        ),
        payload=search_payload,
        media_type="application/json",
        request_fingerprint=sha256(b"root-comment-search-request").hexdigest(),
        page_key="search:root-comment",
    )
    search_lease = claim(database, Dispatch(job_id=search.job_id, epoch=1))
    assert search_lease is not None
    assert CollectionExecutor(collection, sources, StaticFetcher(search_page), store).execute(
        search_lease
    )
    assert not complete(database, search_lease)

    with database() as session:
        detail = session.scalar(
            select(CollectionRun).where(CollectionRun.operation == "fetch_post")
        )
        assert detail is not None
        detail_job_id = detail.job_id
    detail_payload = b'{"post":"video:113","reply_count":2}'
    detail_result = result(
        "AI 根帖",
        2,
        detail_payload,
        external_id="video:113",
        root_id="video:113",
        operation="fetch_post",
    ).model_copy(
        update={
            "adapter_version": "synthetic-root-comment-poc",
            "references": [
                SourceReference(
                    external_id="aid:113",
                    canonical_url="https://www.bilibili.com/video/BV1BVFWeHEaV#reply",
                )
            ],
        }
    )
    detail_page = FetchedPage(
        result=detail_result,
        payload=detail_payload,
        media_type="application/json",
        request_fingerprint=sha256(b"root-comment-detail-request").hexdigest(),
        page_key="post:root-comment",
    )
    detail_lease = claim(database, Dispatch(job_id=detail_job_id, epoch=1))
    assert detail_lease is not None
    assert CollectionExecutor(collection, sources, StaticFetcher(detail_page), store).execute(
        detail_lease
    )
    assert not complete(database, detail_lease)

    with database() as session:
        comments = session.scalar(
            select(CollectionRun).where(CollectionRun.operation == "list_comments")
        )
        assert comments is not None
        assert comments.parent_run_id == detail.id
        assert comments.request_value == "aid:113"
        comments_job_id = comments.job_id
    comments_payload = b'{"comments":["comment:441"],"next":1}'
    comments_result = result(
        "关键词完全不同的观点",
        1,
        comments_payload,
        external_id="comment:441",
        provider_namespace="comment",
        kind="comment",
        root_id="video:113",
        operation="list_comments",
    ).model_copy(
        update={
            "cursor": "1",
            "references": [
                SourceReference(
                    external_id="aid:113/root:441",
                    canonical_url="https://www.bilibili.com/video/av113#reply441",
                )
            ],
        }
    )
    comments_page = FetchedPage(
        result=comments_result,
        payload=comments_payload,
        media_type="application/json",
        request_fingerprint=sha256(b"root-comment-page-request").hexdigest(),
        page_key="comments:root-comment",
    )
    comments_lease = claim(database, Dispatch(job_id=comments_job_id, epoch=1))
    assert comments_lease is not None
    comments_first_fetcher = StaticFetcher(comments_page)
    assert CollectionExecutor(collection, sources, comments_first_fetcher, store).execute(
        comments_lease
    )
    assert not complete(database, comments_lease)
    assert comments_first_fetcher.inputs[0].cursor is None
    assert claim(database, Dispatch(job_id=comments_job_id, epoch=1)) is None

    comments_second_payload = b'{"comments":["comment:443"],"next":2}'
    comments_second_result = result(
        "第二页观点",
        1,
        comments_second_payload,
        external_id="comment:443",
        provider_namespace="comment",
        kind="comment",
        root_id="video:113",
        operation="list_comments",
    ).model_copy(
        update={
            "cursor": "2",
            "references": [
                SourceReference(
                    external_id="aid:113/root:441",
                    canonical_url="https://www.bilibili.com/video/av113#reply441",
                ),
                SourceReference(
                    external_id="aid:113/root:443",
                    canonical_url="https://www.bilibili.com/video/av113#reply443",
                ),
                SourceReference(
                    external_id="aid:113/root:445",
                    canonical_url="https://www.bilibili.com/video/av113#reply445",
                ),
            ],
            "items": [
                *result(
                    "第二页观点",
                    1,
                    comments_second_payload,
                    external_id="comment:443",
                    provider_namespace="comment",
                    kind="comment",
                    root_id="video:113",
                    operation="list_comments",
                ).items,
                *result(
                    "第三个有回复的根评",
                    1,
                    comments_second_payload,
                    external_id="comment:445",
                    provider_namespace="comment",
                    kind="comment",
                    root_id="video:113",
                    operation="list_comments",
                ).items,
            ],
        }
    )
    comments_second_page = FetchedPage(
        result=comments_second_result,
        payload=comments_second_payload,
        media_type="application/json",
        request_fingerprint=sha256(b"root-comment-page-two-request").hexdigest(),
        page_key="comments:root-comment:2",
    )
    comments_second_lease = claim(database, Dispatch(job_id=comments_job_id, epoch=2))
    assert comments_second_lease is not None
    comments_second_fetcher = StaticFetcher(comments_second_page)
    assert CollectionExecutor(collection, sources, comments_second_fetcher, store).execute(
        comments_second_lease
    )
    assert comments_second_fetcher.inputs[0].cursor == "1"
    assert not complete(database, comments_second_lease)

    with database() as session:
        reply_runs = list(
            session.scalars(
                select(CollectionRun)
                .where(CollectionRun.operation == "list_replies")
                .order_by(CollectionRun.request_value)
            )
        )
        assert [run.request_value for run in reply_runs] == [
            "aid:113/root:441",
            "aid:113/root:443",
        ]
        assert {run.parent_run_id for run in reply_runs} == {comments.id}
        replies, second_replies = reply_runs
        replies_job_id = replies.job_id
    replies_payload = b'{"replies":["comment:442"],"next":2}'
    replies_result = result(
        "同意",
        0,
        replies_payload,
        external_id="comment:442",
        provider_namespace="comment",
        kind="reply",
        root_id="video:113",
        parent_id="comment:441",
        operation="list_replies",
    ).model_copy(update={"cursor": "2"})
    replies_page = FetchedPage(
        result=replies_result,
        payload=replies_payload,
        media_type="application/json",
        request_fingerprint=sha256(b"reply-page-request").hexdigest(),
        page_key="replies:root-comment",
    )
    replies_lease = claim(database, Dispatch(job_id=replies_job_id, epoch=1))
    assert replies_lease is not None
    replies_first_fetcher = StaticFetcher(replies_page)
    assert CollectionExecutor(collection, sources, replies_first_fetcher, store).execute(
        replies_lease
    )
    assert not complete(database, replies_lease)
    assert replies_first_fetcher.inputs[0].cursor is None
    assert claim(database, Dispatch(job_id=replies_job_id, epoch=1)) is None

    replies_second_payload = b'{"replies":["comment:444"],"next":3}'
    replies_second_result = result(
        "继续同意",
        0,
        replies_second_payload,
        external_id="comment:444",
        provider_namespace="comment",
        kind="reply",
        root_id="video:113",
        parent_id="comment:441",
        operation="list_replies",
    ).model_copy(update={"cursor": "3"})
    replies_second_page = FetchedPage(
        result=replies_second_result,
        payload=replies_second_payload,
        media_type="application/json",
        request_fingerprint=sha256(b"reply-page-two-request").hexdigest(),
        page_key="replies:root-comment:2",
    )
    replies_second_lease = claim(database, Dispatch(job_id=replies_job_id, epoch=2))
    assert replies_second_lease is not None
    replies_second_fetcher = StaticFetcher(replies_second_page)
    assert CollectionExecutor(collection, sources, replies_second_fetcher, store).execute(
        replies_second_lease
    )
    assert replies_second_fetcher.inputs[0].cursor == "2"
    assert not complete(database, replies_second_lease)

    second_replies_payload = b'{"replies":["comment:446"]}'
    second_replies_result = result(
        "第二个父级的回复",
        0,
        second_replies_payload,
        external_id="comment:446",
        provider_namespace="comment",
        kind="reply",
        root_id="video:113",
        parent_id="comment:443",
        operation="list_replies",
    )
    second_replies_page = FetchedPage(
        result=second_replies_result,
        payload=second_replies_payload,
        media_type="application/json",
        request_fingerprint=sha256(b"second-root-reply-request").hexdigest(),
        page_key="replies:second-root",
    )
    second_replies_lease = claim(database, Dispatch(job_id=second_replies.job_id, epoch=1))
    assert second_replies_lease is not None
    assert CollectionExecutor(
        collection,
        sources,
        StaticFetcher(second_replies_page),
        store,
    ).execute(second_replies_lease)
    assert not complete(database, second_replies_lease)

    inbox = {item.external_id: item for item in ContentService(database).inbox(20, None).items}
    assert set(inbox) == {
        "video:113",
        "comment:441",
        "comment:442",
        "comment:443",
        "comment:444",
        "comment:445",
        "comment:446",
    }
    assert inbox["comment:441"].relation_status == "resolved"
    assert [match.monitor_title for match in inbox["comment:441"].matches] == ["评论上下文"]
    assert inbox["comment:442"].relation_status == "resolved"
    assert inbox["comment:442"].parent_external_id == "comment:441"
    assert [match.monitor_title for match in inbox["comment:442"].matches] == ["评论上下文"]
    assert inbox["comment:446"].parent_external_id == "comment:443"
    assert [match.monitor_title for match in inbox["comment:446"].matches] == ["评论上下文"]
    comment_run = collection.run(comments.id)
    assert comment_run.outcome == "partial"
    assert comment_run.stop_reason == "page_limit"
    reply_run = collection.run(replies.id)
    assert reply_run.outcome == "partial"
    assert reply_run.stop_reason == "page_limit"
    second_reply_run = collection.run(second_replies.id)
    assert second_reply_run.outcome == "ok"
    assert second_reply_run.stop_reason is None
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 5
        assert session.scalar(select(func.count()).select_from(Job)) == 5
        assert session.scalar(select(func.count()).select_from(Outbox)) == 7
        assert session.scalar(select(func.count()).select_from(RawPage)) == 7
        assert session.scalar(select(func.count()).select_from(CollectionCheckpoint)) == 7
        assert (
            session.scalar(
                select(func.count())
                .select_from(CollectionRun)
                .where(
                    CollectionRun.parent_run_id == comments.id,
                    CollectionRun.operation == "list_replies",
                )
            )
            == 2
        )
        usage = session.scalar(select(CollectionBudgetUsage))
        assert usage is not None and usage.reserved_requests == 12
        persisted_comments = session.get(CollectionRun, comments.id)
        persisted_replies = session.get(CollectionRun, replies.id)
        persisted_second_replies = session.get(CollectionRun, second_replies.id)
        assert persisted_comments is not None and persisted_comments.reserved_requests == 4
        assert persisted_replies is not None and persisted_replies.reserved_requests == 4
        assert (
            persisted_second_replies is not None and persisted_second_replies.reserved_requests == 2
        )
        comment_job = session.get(Job, comments.job_id)
        reply_job = session.get(Job, replies.job_id)
        assert comment_job is not None and comment_job.epoch == 2 and comment_job.attempts == 2
        assert reply_job is not None and reply_job.epoch == 2 and reply_job.attempts == 2
        comment_attempts = list(
            session.scalars(select(Attempt).where(Attempt.job_id == comments.job_id))
        )
        reply_attempts = list(
            session.scalars(select(Attempt).where(Attempt.job_id == replies.job_id))
        )
        assert {attempt.outcome for attempt in comment_attempts} == {
            "page_committed",
            "succeeded",
        }
        assert {attempt.outcome for attempt in reply_attempts} == {
            "page_committed",
            "succeeded",
        }


def test_comment_page_budget_exhaustion_keeps_committed_page_without_next_epoch(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    created = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "评论分页预算",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "schedule": {"interval_minutes": 1440, "retention_days": 7},
                "budget": {"daily_requests": 8, "content_purchase_cost": 0},
            }
        )
    )
    active = monitors.change_state(
        created.id,
        MonitorStateChange(expected_version=created.current_version),
        "active",
    )
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    search = create_collection_run(
        collection,
        active.id,
        active.current_version,
        "comment-page-budget",
    )
    create_collection_run(
        collection,
        active.id,
        active.current_version,
        "comment-page-budget-blocker",
    )
    create_collection_run(
        collection,
        active.id,
        active.current_version,
        "comment-page-budget-second-blocker",
    )
    create_collection_run(
        collection,
        active.id,
        active.current_version,
        "comment-page-budget-third-blocker",
    )

    def reference_page(operation, request, reference, page_key):
        payload = f'{{"reference":"{reference}"}}'.encode()
        return FetchedPage(
            result=SourceResult(
                source="bilibili",
                adapter_version="synthetic-page-budget-poc",
                operation=operation,
                status="ok",
                observed_at=utcnow(),
                references=[
                    SourceReference(
                        external_id=reference,
                        canonical_url="https://www.bilibili.com/video/BV1BVFWeHEaV",
                    )
                ],
                response_bytes=len(payload),
                response_sha256=sha256(payload).hexdigest(),
            ),
            payload=payload,
            media_type="application/json",
            request_fingerprint=sha256(request).hexdigest(),
            page_key=page_key,
        )

    search_lease = claim(database, Dispatch(job_id=search.job_id, epoch=1))
    assert search_lease is not None
    assert CollectionExecutor(
        collection,
        sources,
        StaticFetcher(
            reference_page(
                "search_posts",
                b"comment-page-budget-search",
                "bvid:BV1BVFWeHEaV",
                "search:comment-page-budget",
            )
        ),
        store,
    ).execute(search_lease)
    with database() as session:
        detail = session.scalar(
            select(CollectionRun).where(CollectionRun.operation == "fetch_post")
        )
        assert detail is not None
    detail_lease = claim(database, Dispatch(job_id=detail.job_id, epoch=1))
    assert detail_lease is not None
    assert CollectionExecutor(
        collection,
        sources,
        StaticFetcher(
            reference_page(
                "fetch_post",
                b"comment-page-budget-detail",
                "aid:113",
                "post:comment-page-budget",
            )
        ),
        store,
    ).execute(detail_lease)
    with database() as session:
        comments = session.scalar(
            select(CollectionRun).where(CollectionRun.operation == "list_comments")
        )
        assert comments is not None

    payload = b'{"comments":[],"next":1}'
    page = FetchedPage(
        result=SourceResult(
            source="bilibili",
            adapter_version="synthetic-page-budget-poc",
            operation="list_comments",
            status="ok",
            observed_at=utcnow(),
            cursor="1",
            response_bytes=len(payload),
            response_sha256=sha256(payload).hexdigest(),
        ),
        payload=payload,
        media_type="application/json",
        request_fingerprint=sha256(b"comment-page-budget-comments").hexdigest(),
        page_key="comments:comment-page-budget",
    )
    comments_lease = claim(database, Dispatch(job_id=comments.job_id, epoch=1))
    assert comments_lease is not None
    assert CollectionExecutor(collection, sources, StaticFetcher(page), store).execute(
        comments_lease
    )
    assert not complete(database, comments_lease)

    with database() as session:
        persisted = session.get(CollectionRun, comments.id)
        job = session.get(Job, comments.job_id)
        usage = session.scalar(select(CollectionBudgetUsage))
        assert persisted is not None
        assert persisted.pages_count == 1 and persisted.reserved_requests == 2
        assert persisted.state == "completed" and persisted.outcome == "partial"
        assert persisted.stop_reason == "page_budget_exhausted"
        assert job is not None and job.status == "succeeded" and job.epoch == 1
        assert usage is not None and usage.reserved_requests == 7
        assert session.scalar(select(func.count()).select_from(Outbox)) == 6
        assert (
            session.scalar(
                select(func.count())
                .select_from(CollectionCheckpoint)
                .where(CollectionCheckpoint.run_id == comments.id)
            )
            == 1
        )


def test_reference_expansion_stops_at_budget_without_creating_a_detail_job(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    created_monitor = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "详情预算",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "schedule": {"interval_minutes": 1440, "retention_days": 7},
                "budget": {"daily_requests": 8, "content_purchase_cost": 0},
            }
        )
    )
    active = monitors.change_state(
        created_monitor.id,
        MonitorStateChange(expected_version=created_monitor.current_version),
        "active",
    )
    collection = CollectionService(database, sources, evidence_configured=True)

    def create(key):
        return collection.create_run(
            CollectionRunInput(
                monitor_id=active.id,
                expected_version=active.current_version,
                source="bilibili",
                request_value="AI",
                since="2026-09-14T00:00:00Z",
                until="2026-09-15T00:00:00Z",
                idempotency_key=key,
                policy_version="synthetic-policy-v1",
                retention_days=7,
                ingestion_mode="live",
            )
        )

    parent = create("budget-parent")
    create("budget-competing-run")
    create("budget-second-competing-run")
    create("budget-third-competing-run")
    create("budget-fourth-competing-run")
    create("budget-fifth-competing-run")
    create("budget-sixth-competing-run")
    create("budget-seventh-competing-run")
    payload = b'{"references":["BV1BVFWeHEaV"]}'
    page = FetchedPage(
        result=SourceResult(
            source="bilibili",
            adapter_version="synthetic-reference-poc",
            operation="search_posts",
            status="ok",
            observed_at=utcnow(),
            references=[
                SourceReference(
                    external_id="bvid:BV1BVFWeHEaV",
                    canonical_url="https://www.bilibili.com/video/BV1BVFWeHEaV",
                )
            ],
            response_bytes=len(payload),
            response_sha256=sha256(payload).hexdigest(),
        ),
        payload=payload,
        media_type="application/json",
        request_fingerprint=sha256(b"budget-reference-request").hexdigest(),
        page_key="search:budget-reference",
    )
    lease = claim(database, Dispatch(job_id=parent.job_id, epoch=1))
    assert lease is not None
    assert CollectionExecutor(collection, sources, StaticFetcher(page), MemoryStore()).execute(
        lease
    )

    persisted = collection.run(parent.id)
    assert persisted.outcome == "partial"
    assert persisted.stop_reason == "detail_budget_exhausted"
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 8
        assert session.scalar(select(func.count()).select_from(Job)) == 8


def test_pausing_monitor_before_page_boundary_prevents_source_fetch(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    active = monitor(monitors, "暂停边界", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="pause-before-fetch",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None
    monitors.change_state(
        active.id,
        MonitorStateChange(expected_version=active.current_version),
        "paused",
    )
    payload = b'{"must":"not fetch"}'
    fetcher = StaticFetcher(
        FetchedPage(
            result=result("AI", 0, payload),
            payload=payload,
            media_type="application/json",
            request_fingerprint=sha256(b"pause-before-fetch").hexdigest(),
            page_key="search:pause-before-fetch",
        )
    )
    assert not CollectionExecutor(collection, sources, fetcher, MemoryStore()).execute(lease)
    assert not complete(database, lease)
    persisted = collection.run(created.id)
    assert persisted.state == "cancelled"
    assert persisted.outcome == "partial"
    assert persisted.stop_reason == "monitor_inactive"
    assert fetcher.calls == 0
    with database() as session:
        job = session.get(Job, created.job_id)
        assert job is not None and job.status == "cancelled"


def test_detail_revocation_at_search_commit_creates_no_followup(database):
    sources = SearchOnlySources()
    active = monitor(MonitorService(database, sources), "详情撤权", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="detail-revoked",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    job_lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert job_lease is not None
    execution = collection.claim_for_job(job_lease)
    assert execution is not None
    payload = b'{"references":["BV1BVFWeHEaV"]}'
    committed = collection.commit_page(
        PageCommitInput(
            run_id=created.id,
            fencing_token=execution.fencing_token,
            page_key="search:detail-revoked",
            request_fingerprint=sha256(b"detail-revoked-request").hexdigest(),
            media_type="application/json",
            payload=payload,
            retention_until=utcnow() + timedelta(days=7),
            policy_version="synthetic-policy-v1",
            result=SourceResult(
                source="bilibili",
                adapter_version="synthetic-reference-poc",
                operation="search_posts",
                status="ok",
                observed_at=utcnow(),
                references=[
                    SourceReference(
                        external_id="bvid:BV1BVFWeHEaV",
                        canonical_url="https://www.bilibili.com/video/BV1BVFWeHEaV",
                    )
                ],
                response_bytes=len(payload),
                response_sha256=sha256(payload).hexdigest(),
            ),
        ),
        MemoryStore(),
        job_lease,
    )
    assert committed.followup_run_count == 0
    persisted = collection.run(created.id)
    assert persisted.outcome == "partial"
    assert persisted.stop_reason == "detail_not_eligible"
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 1
        assert session.scalar(select(func.count()).select_from(Job)) == 1


def test_message_redelivery_after_atomic_page_commit_cannot_refetch(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "提交后恢复", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="crash-after-commit",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    payload = b'{"page":"committed-before-crash"}'
    fetcher = StaticFetcher(
        FetchedPage(
            result=result("AI 已提交", 0, payload),
            payload=payload,
            media_type="application/json",
            request_fingerprint=sha256(b"crash-after-commit-request").hexdigest(),
            page_key="search:crash-after-commit",
        )
    )
    executor = CollectionExecutor(collection, sources, fetcher, MemoryStore())
    first = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert first is not None and executor.execute(first)
    with database() as session:
        job = session.get(Job, created.job_id)
        assert job is not None and job.status == "succeeded"
    assert reconcile(database) == 0
    assert claim(database, Dispatch(job_id=created.job_id, epoch=1)) is None
    assert not executor.execute(first)
    assert not complete(database, first)
    assert fetcher.calls == 1


def test_permanent_source_failure_atomically_fails_run_and_job(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "永久失败", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = create_collection_run(
        collection, active.id, active.current_version, "permanent-source-failure"
    )
    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None

    should_complete = CollectionExecutor(
        collection,
        sources,
        StaticFetcher(failed_page("schema_changed")),
        MemoryStore(),
    ).execute(lease)
    assert not should_complete
    assert not complete(database, lease)

    with database() as session:
        run = session.get(CollectionRun, created.id)
        job = session.get(Job, created.job_id)
        assert run is not None and job is not None
        assert run.state == "failed" and run.outcome == "failed"
        assert run.stop_reason == "schema_changed"
        assert job.status == "failed"
        assert job.completed_at is not None


def test_rate_limit_retry_reserves_budget_and_second_attempt_commits_once(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "限流恢复", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = create_collection_run(
        collection, active.id, active.current_version, "rate-limit-retry"
    )
    payload = b'{"page":"after-rate-limit"}'
    success = FetchedPage(
        result=result("AI 限流后成功", 0, payload),
        payload=payload,
        media_type="application/json",
        request_fingerprint=sha256(b"after-rate-limit").hexdigest(),
        page_key="search:after-rate-limit",
    )
    fetcher = SequenceFetcher([failed_page("rate_limited", 12), success])
    executor = CollectionExecutor(collection, sources, fetcher, MemoryStore())
    before = utcnow()
    first = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert first is not None
    assert not executor.execute(first)

    with database.begin() as session:
        run = session.get(CollectionRun, created.id)
        job = session.get(Job, created.job_id)
        usage = session.scalar(
            select(CollectionBudgetUsage).where(
                CollectionBudgetUsage.monitor_version_id == created.monitor_version_id
            )
        )
        assert run is not None and job is not None and usage is not None
        assert run.state == "queued" and run.reserved_requests == 2
        assert run.stop_reason == "rate_limited"
        assert job.status == "queued" and job.epoch == 2
        assert (
            before + timedelta(seconds=11) <= job.available_at <= utcnow() + timedelta(seconds=13)
        )
        assert usage.reserved_requests == 2
        assert session.scalar(select(func.count()).select_from(Outbox)) == 2
        session.execute(update(Job).where(Job.id == created.job_id).values(available_at=utcnow()))

    second = claim(database, Dispatch(job_id=created.job_id, epoch=2))
    assert second is not None and executor.execute(second)
    assert not complete(database, second)
    with database() as session:
        run = session.get(CollectionRun, created.id)
        job = session.get(Job, created.job_id)
        assert run is not None and job is not None
        assert run.state == "completed" and run.pages_count == 1
        assert job.status == "succeeded" and job.attempts == 2
        assert session.scalar(select(func.count()).select_from(RawPage)) == 1
    assert fetcher.calls == 2


def test_comment_retry_reserves_full_anonymous_session_request_cost(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "评论重试预算", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    parent = create_collection_run(
        collection, active.id, active.current_version, "comment-retry-parent"
    )
    created = create_collection_run(
        collection, active.id, active.current_version, "comment-rate-limit-retry"
    )
    with database.begin() as session:
        run = session.get(CollectionRun, created.id)
        usage = session.scalar(select(CollectionBudgetUsage))
        assert run is not None and usage is not None
        run.parent_run_id = parent.id
        run.operation = "list_comments"
        run.request_value = "aid:113"
        run.reserved_requests = 2
        usage.reserved_requests = 3

    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None
    assert not CollectionExecutor(
        collection,
        sources,
        StaticFetcher(failed_page("rate_limited", 12)),
        MemoryStore(),
    ).execute(lease)

    with database() as session:
        run = session.get(CollectionRun, created.id)
        usage = session.scalar(select(CollectionBudgetUsage))
        assert run is not None and usage is not None
        assert run.state == "queued" and run.reserved_requests == 4
        assert usage.reserved_requests == 5


def test_transient_failure_stops_after_three_attempts_without_fourth_outbox(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "重试上限", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = create_collection_run(collection, active.id, active.current_version, "retry-limit")
    executor = CollectionExecutor(
        collection,
        sources,
        StaticFetcher(failed_page("network_error")),
        MemoryStore(),
    )

    for epoch in (1, 2, 3):
        with database.begin() as session:
            session.execute(
                update(Job).where(Job.id == created.job_id).values(available_at=utcnow())
            )
        lease = claim(database, Dispatch(job_id=created.job_id, epoch=epoch))
        assert lease is not None
        assert not executor.execute(lease)

    with database() as session:
        run = session.get(CollectionRun, created.id)
        job = session.get(Job, created.job_id)
        assert run is not None and job is not None
        assert run.state == "failed" and run.stop_reason == "network_error_retry_exhausted"
        assert run.reserved_requests == 3
        assert job.status == "failed" and job.attempts == 3 and job.epoch == 3
        assert session.scalar(select(func.count()).select_from(Outbox)) == 3


def test_retry_budget_exhaustion_atomically_fails_without_retry_outbox(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    created_monitor = monitors.create_monitor(
        MonitorInput.model_validate(
            {
                "title": "重试预算",
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "schedule": {"interval_minutes": 1440, "retention_days": 7},
                "budget": {"daily_requests": 8, "content_purchase_cost": 0},
            }
        )
    )
    active = monitors.change_state(
        created_monitor.id,
        MonitorStateChange(expected_version=created_monitor.current_version),
        "active",
    )
    collection = CollectionService(database, sources, evidence_configured=True)
    target = create_collection_run(
        collection, active.id, active.current_version, "retry-budget-target"
    )
    for index in range(7):
        create_collection_run(
            collection,
            active.id,
            active.current_version,
            f"retry-budget-competing-{index}",
        )
    lease = claim(database, Dispatch(job_id=target.job_id, epoch=1))
    assert lease is not None
    assert not CollectionExecutor(
        collection,
        sources,
        StaticFetcher(failed_page("rate_limited", 12)),
        MemoryStore(),
    ).execute(lease)

    with database() as session:
        run = session.get(CollectionRun, target.id)
        job = session.get(Job, target.job_id)
        usage = session.scalar(
            select(CollectionBudgetUsage).where(
                CollectionBudgetUsage.monitor_version_id == target.monitor_version_id
            )
        )
        assert run is not None and job is not None and usage is not None
        assert run.state == "failed" and run.stop_reason == "retry_budget_exhausted"
        assert job.status == "failed" and job.epoch == 1
        assert usage.reserved_requests == 8
        assert session.scalar(select(func.count()).select_from(Outbox)) == 8


def test_missing_evidence_configuration_creates_no_run_or_job(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "无对象存储", ["AI"])
    collection = CollectionService(database, sources)
    with pytest.raises(AppError, match="evidence_store_not_configured"):
        collection.create_run(
            CollectionRunInput(
                monitor_id=active.id,
                expected_version=active.current_version,
                source="bilibili",
                request_value="AI",
                since="2026-09-14T00:00:00Z",
                until="2026-09-15T00:00:00Z",
                idempotency_key="missing-evidence",
                policy_version="synthetic-policy-v1",
                retention_days=7,
                ingestion_mode="live",
            )
        )
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 0
        assert session.scalar(select(func.count()).select_from(Job)) == 0
        assert session.scalar(select(func.count()).select_from(Outbox)) == 0


def test_idempotent_replay_returns_committed_run_after_admission_is_revoked(database):
    admitted = AdmittedSources()
    active = monitor(MonitorService(database, admitted), "幂等重放", ["AI"])
    data = CollectionRunInput(
        monitor_id=active.id,
        expected_version=active.current_version,
        source="bilibili",
        request_value="AI",
        since="2026-09-14T00:00:00Z",
        until="2026-09-15T00:00:00Z",
        idempotency_key="replay-after-revocation",
        policy_version="synthetic-policy-v1",
        retention_days=7,
        ingestion_mode="live",
    )
    created = CollectionService(database, admitted, evidence_configured=True).create_run(data)

    replayed = CollectionService(database, RevokedSources()).create_run(data)
    assert replayed.id == created.id and replayed.job_id == created.job_id
    with pytest.raises(AppError, match="idempotency_conflict"):
        CollectionService(database, RevokedSources()).create_run(
            data.model_copy(update={"retention_days": 8})
        )
    with pytest.raises(AppError, match="idempotency_conflict"):
        CollectionService(database, RevokedSources()).create_run(
            data.model_copy(update={"ingestion_mode": "backfill"})
        )
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 1
        assert session.scalar(select(func.count()).select_from(Job)) == 1


def test_concurrent_collection_run_creation_has_one_intent(database):
    from concurrent.futures import ThreadPoolExecutor

    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "并发幂等", ["AI"])
    data = CollectionRunInput(
        monitor_id=active.id,
        expected_version=active.current_version,
        source="bilibili",
        request_value="AI",
        since="2026-09-14T00:00:00Z",
        until="2026-09-15T00:00:00Z",
        idempotency_key="concurrent-collection-run",
        policy_version="synthetic-policy-v1",
        retention_days=7,
        ingestion_mode="live",
    )

    def create(_):
        return CollectionService(database, sources, evidence_configured=True).create_run(data)

    with ThreadPoolExecutor(max_workers=4) as pool:
        runs = list(pool.map(create, range(8)))
    assert len({run.id for run in runs}) == 1
    assert len({run.job_id for run in runs}) == 1
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 1
        assert session.scalar(select(func.count()).select_from(Job)) == 1
        assert session.scalar(select(func.count()).select_from(Outbox)) == 1


def test_executor_rechecks_source_eligibility_before_fetch(database):
    admitted = AdmittedSources()
    active = monitor(MonitorService(database, admitted), "撤权", ["AI"])
    collection = CollectionService(database, admitted, evidence_configured=True)
    created = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="revoked-before-fetch",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    payload = b'{"must":"not be fetched"}'
    fetcher = StaticFetcher(
        FetchedPage(
            result=result("AI", 0, payload),
            payload=payload,
            media_type="application/json",
            request_fingerprint=sha256(b"must-not-fetch").hexdigest(),
            page_key="search:must-not-fetch",
        )
    )
    lease = claim(database, Dispatch(job_id=created.job_id, epoch=1))
    assert lease is not None

    assert not CollectionExecutor(collection, RevokedSources(), fetcher, MemoryStore()).execute(
        lease
    )
    persisted = collection.run(created.id)
    assert persisted.state == "failed" and persisted.stop_reason == "source_not_eligible"
    assert fetcher.calls == 0
    with database() as session:
        job = session.get(Job, created.job_id)
        assert job is not None and job.status == "failed"


def test_inbox_only_returns_matched_content_with_latest_observation(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    active = monitor(monitors, "AI 观察", ["AI"])
    run_and_commit(
        collection,
        store,
        active.id,
        active.current_version,
        "matched",
        "AI 首次观察",
        0,
    )
    run_and_commit(
        collection,
        store,
        active.id,
        active.current_version,
        "unmatched",
        "与主题无关",
        None,
        external_id="BV1UNMATCHED",
    )

    page = ContentService(database).inbox(limit=20, cursor=None)

    assert len(page.items) == 1
    assert page.items[0].provider_namespace == "video"
    assert page.items[0].external_id == "BV1BVFWeHEaV"
    assert page.items[0].root_external_id == "BV1BVFWeHEaV"
    assert page.items[0].parent_external_id is None
    assert page.items[0].relation_status == "root"
    assert page.items[0].text == "AI 首次观察"
    assert page.items[0].reply_count == 0
    assert [match.monitor_title for match in page.items[0].matches] == ["AI 观察"]
    assert page.next_cursor is None


def test_inbox_review_is_scoped_to_one_monitor_match(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    first = monitor(monitors, "AI 主题", ["AI"])
    second = monitor(monitors, "观察主题", ["AI"])
    for active, key in ((first, "match-first"), (second, "match-second")):
        run_and_commit(
            collection,
            store,
            active.id,
            active.current_version,
            key,
            "AI 观察",
            0,
        )

    initial = ContentService(database).inbox(limit=20, cursor=None)
    assert len(initial.items) == 1
    matches = {match.monitor_title: match for match in initial.items[0].matches}
    assert set(matches) == {"AI 主题", "观察主题"}

    from monitors.schemas import MonitorMatchReviewInput

    ignored = monitors.review_match(
        matches["AI 主题"].id,
        MonitorMatchReviewInput(review_state="ignored"),
    )
    assert ignored.review_state == "ignored"

    default_page = ContentService(database).inbox(limit=20, cursor=None)
    assert [match.monitor_title for match in default_page.items[0].matches] == ["观察主题"]
    assert (
        not ContentService(database)
        .inbox(
            limit=20,
            cursor=None,
            monitor_id=first.id,
        )
        .items
    )
    ignored_page = ContentService(database).inbox(
        limit=20,
        cursor=None,
        monitor_id=first.id,
        review_state="ignored",
    )
    assert [match.monitor_title for match in ignored_page.items[0].matches] == ["AI 主题"]
    assert (
        not ContentService(database)
        .inbox(
            limit=20,
            cursor=None,
            source="x",
        )
        .items
    )
    assert (
        not ContentService(database)
        .inbox(
            limit=20,
            cursor=None,
            discovered_since=utcnow() + timedelta(days=1),
        )
        .items
    )

    restored = monitors.review_match(
        matches["AI 主题"].id,
        MonitorMatchReviewInput(review_state="new"),
    )
    assert restored.review_state == "new"
    with database() as session:
        assert (
            session.scalar(
                select(func.count())
                .select_from(Audit)
                .where(Audit.action == "monitor_match_reviewed")
            )
            == 2
        )
    with pytest.raises(AppError) as missing:
        monitors.review_match(
            uuid4(),
            MonitorMatchReviewInput(review_state="ignored"),
        )
    assert missing.value.code == "monitor_match_not_found"
    assert missing.value.status == 404


def test_provider_namespace_is_part_of_content_identity(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    active = monitor(monitors, "命名空间", ["AI"])
    for key, namespace in (("namespace-a", "video"), ("namespace-b", "article")):
        run_and_commit(
            collection,
            store,
            active.id,
            active.current_version,
            key,
            "AI",
            0,
            external_id="same-provider-id",
            provider_namespace=namespace,
        )

    with database() as session:
        assert session.scalar(select(func.count()).select_from(Content)) == 2
    first_page = ContentService(database).inbox(limit=1, cursor=None)
    assert len(first_page.items) == 1
    assert isinstance(first_page.next_cursor, str)
    second_page = ContentService(database).inbox(limit=1, cursor=first_page.next_cursor)
    assert len(second_page.items) == 1
    assert second_page.items[0].id != first_page.items[0].id
    assert second_page.next_cursor is None


def test_ambiguous_root_identity_stays_unresolved_and_does_not_inherit_match(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    active = monitor(monitors, "歧义根关系", ["AI"])
    for key, namespace in (("ambiguous-root-a", "video"), ("ambiguous-root-b", "article")):
        run_and_commit(
            collection,
            store,
            active.id,
            active.current_version,
            key,
            "AI 根帖",
            1,
            external_id="same-root-id",
            provider_namespace=namespace,
        )
    run_and_commit(
        collection,
        store,
        active.id,
        active.current_version,
        "ambiguous-comment",
        "不含主题词",
        0,
        external_id="comment:99",
        provider_namespace="comment",
        kind="comment",
        root_id="same-root-id",
    )

    with database() as session:
        comment = session.scalar(select(Content).where(Content.external_id == "comment:99"))
        assert comment is not None and comment.relation_status == "unresolved"
        assert (
            session.scalar(
                select(func.count())
                .select_from(MonitorMatch)
                .where(MonitorMatch.content_id == comment.id)
            )
            == 0
        )
    assert len(ContentService(database).inbox(limit=20, cursor=None).items) == 2


def test_reply_with_missing_parent_stays_unresolved_and_does_not_inherit_match(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    active = monitor(monitors, "缺失父评论", ["AI"])
    run_and_commit(
        collection,
        store,
        active.id,
        active.current_version,
        "reply-root",
        "AI 根帖",
        1,
        external_id="video:1",
    )
    run_and_commit(
        collection,
        store,
        active.id,
        active.current_version,
        "missing-parent-reply",
        "同意",
        0,
        external_id="comment:2",
        provider_namespace="comment",
        kind="reply",
        root_id="video:1",
        parent_id="comment:missing",
    )

    with database() as session:
        reply = session.scalar(select(Content).where(Content.external_id == "comment:2"))
        assert reply is not None and reply.relation_status == "unresolved"
        assert (
            session.scalar(
                select(func.count())
                .select_from(MonitorMatch)
                .where(MonitorMatch.content_id == reply.id)
            )
            == 0
        )
    inbox = ContentService(database).inbox(limit=20, cursor=None).items
    assert [item.external_id for item in inbox] == ["video:1"]


def test_top_level_comment_without_parent_remains_unresolved(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    active = monitor(monitors, "评论", ["AI"])
    run_and_commit(
        collection,
        MemoryStore(),
        active.id,
        active.current_version,
        "top-level-comment",
        "AI 评论",
        0,
        external_id="comment:10",
        provider_namespace="comment",
        kind="comment",
        root_id="video:1",
    )

    item = ContentService(database).inbox(limit=20, cursor=None).items[0]
    assert item.kind == "comment"
    assert item.parent_external_id is None
    assert item.relation_status == "unresolved"


def test_invalid_page_is_rejected_before_object_upload(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    active = monitor(monitors, "预检", ["AI"])
    started = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="preflight-reject",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    job_lease = claim(database, Dispatch(job_id=started.job_id, epoch=1))
    assert job_lease is not None
    lease = collection.claim_for_job(job_lease)
    assert lease is not None
    payload = b'{"page":"wrong-source"}'
    page = PageCommitInput(
        run_id=started.id,
        fencing_token=lease.fencing_token,
        page_key="first",
        request_fingerprint=sha256(b"wrong-source").hexdigest(),
        media_type="application/json",
        payload=payload,
        retention_until=utcnow() + timedelta(days=7),
        policy_version="synthetic-policy-v1",
        result=result("AI", 0, payload).model_copy(update={"source": "bluesky"}),
    )

    with pytest.raises(AppError, match="collection_result_mismatch"):
        collection.commit_page(page, store, job_lease)

    assert store.objects == {}


def test_withdrawal_tombstone_rejects_recollected_page_before_object_upload(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    active = monitor(monitors, "删除防复活", ["AI"])
    now = utcnow()
    with database.begin() as session:
        session.add(
            ContentWithdrawalRecord(
                id=uuid4(),
                source="bilibili",
                provider_namespace="video",
                external_id="BV1BVFWeHEaV",
                visibility="deleted",
                requested_at=now,
                updated_at=now,
            )
        )
    store = MemoryStore()

    with pytest.raises(AppError, match="content_withdrawn"):
        run_and_commit(
            collection,
            store,
            active.id,
            active.current_version,
            "withdrawn-content-recollection",
            "不得复活的合成正文",
            0,
        )

    assert store.objects == {}


def test_withdrawal_race_after_upload_removes_object_and_rejects_commit(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    active = monitor(monitors, "删除竞态", ["AI"])
    store = WithdrawalDuringPutStore(database)

    with pytest.raises(AppError, match="content_withdrawn"):
        run_and_commit(
            collection,
            store,
            active.id,
            active.current_version,
            "withdrawal-during-upload",
            "不得落库的合成正文",
            0,
        )

    assert store.objects == {}
    with database() as session:
        assert session.scalar(select(func.count()).select_from(RawPage)) == 0
        assert session.scalar(select(func.count()).select_from(Content)) == 0


def test_withdrawal_race_tracks_failed_compensation_for_reconciliation(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    active = monitor(monitors, "删除补偿", ["AI"])
    store = WithdrawalDuringPutStore(database, fail_delete=True)

    with pytest.raises(AppError, match="content_withdrawn_evidence_cleanup_failed"):
        run_and_commit(
            collection,
            store,
            active.id,
            active.current_version,
            "withdrawal-cleanup-retry",
            "需对账删除的合成正文",
            0,
        )

    with database() as session:
        raw_page = session.scalar(select(RawPage))
        assert raw_page is not None
        assert raw_page.object_state == "failed"
        assert raw_page.cleanup_attempts == 1
        assert session.scalar(select(func.count()).select_from(Content)) == 0

    store.fail_delete = False
    assert EvidenceDeletionService(database, store).reconcile(1).deleted == 1
    assert store.objects == {}


def test_old_fence_cannot_commit_and_stale_upload_is_compensated(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    active = monitor(monitors, "隔离", ["AI"])
    started = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            request_value="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="old-fence",
            policy_version="synthetic-policy-v1",
            retention_days=7,
            ingestion_mode="live",
        )
    )
    job_lease = claim(database, Dispatch(job_id=started.job_id, epoch=1))
    assert job_lease is not None
    lease = collection.claim_for_job(job_lease)
    assert lease is not None
    store = FenceChangingStore(database, started.id)
    payload = b'{"page":"old-fence"}'
    page = PageCommitInput(
        run_id=started.id,
        fencing_token=lease.fencing_token,
        page_key="first",
        request_fingerprint=sha256(b"old-fence").hexdigest(),
        media_type="application/json",
        payload=payload,
        retention_until=utcnow() + timedelta(days=7),
        policy_version="synthetic-policy-v1",
        result=result("AI", None, payload),
    )
    with pytest.raises(AppError, match="stale_collection_lease"):
        collection.commit_page(page, store, job_lease)
    assert store.objects == {}
    with database() as session:
        assert session.scalar(select(func.count()).select_from(RawPage)) == 0
