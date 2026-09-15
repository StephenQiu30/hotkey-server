from datetime import UTC, datetime, timedelta
from hashlib import sha256

import pytest
from sqlalchemy import func, select

from collection.execution import CollectionExecutor
from collection.models import CollectionCheckpoint, CollectionRun
from collection.schemas import CollectionRunInput, PageCommitInput
from collection.services import CollectionService
from contents.models import Content, ContentObservation, ContentVersion
from contents.services import ContentService
from core.clock import utcnow
from core.errors import AppError
from evidence.contracts import StoredObject
from evidence.models import RawPage
from jobs.contracts import Dispatch
from jobs.execution import claim, complete, reconcile
from jobs.models import Job, JobResult, Outbox
from monitors.models import MonitorMatch
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.contracts import FetchedPage
from sources.schemas import SocialObject, SourceResult
from sources.services import SourceService

pytestmark = pytest.mark.integration


class AdmittedSources(SourceService):
    def activation_issues(self, source_ids):
        return []


class RevokedSources(SourceService):
    def activation_issues(self, source_ids):
        return ["source_not_eligible"]


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


class StaticFetcher:
    def __init__(self, page: FetchedPage):
        self.page = page
        self.calls = 0

    def fetch(self, data):
        self.calls += 1
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
) -> SourceResult:
    return SourceResult(
        source="bilibili",
        adapter_version="synthetic-transaction-poc",
        operation="search_posts",
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
            query_variant="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key=key,
            policy_version="synthetic-policy-v1",
            retention_days=7,
        )
    )
    lease = service.claim_for_job(started.job_id, 1)
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
    return service.commit_page(page, store), page


def test_page_commit_is_idempotent_and_preserves_versions_matches_and_zero(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    store = MemoryStore()
    first = monitor(monitors, "主题一", ["AI"])
    committed, page = run_and_commit(
        collection, store, first.id, first.current_version, "run-one", "AI 初版", None
    )
    duplicate = collection.commit_page(page, store)
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
            query_variant="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="executor-one-page",
            policy_version="synthetic-policy-v1",
            retention_days=7,
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
    assert complete(database, lease)

    persisted = collection.run(created.id)
    assert persisted.state == "completed" and persisted.outcome == "ok"
    assert persisted.pages_count == 1 and persisted.items_count == 1
    assert fetcher.calls == 1
    with database() as session:
        assert session.scalar(select(func.count()).select_from(Job)) == 1
        assert session.scalar(select(func.count()).select_from(Outbox)) == 1
        assert session.scalar(select(func.count()).select_from(JobResult)) == 0


def test_worker_crash_after_page_commit_completes_job_without_refetch(database):
    sources = AdmittedSources()
    active = monitor(MonitorService(database, sources), "提交后恢复", ["AI"])
    collection = CollectionService(database, sources, evidence_configured=True)
    created = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            query_variant="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="crash-after-commit",
            policy_version="synthetic-policy-v1",
            retention_days=7,
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
    with database.begin() as session:
        job = session.get(Job, created.job_id)
        assert job is not None
        job.lease_until = utcnow() - timedelta(seconds=1)
    assert reconcile(database) == 1
    with database.begin() as session:
        job = session.get(Job, created.job_id)
        assert job is not None
        job.available_at = utcnow() - timedelta(seconds=1)
    replacement = claim(database, Dispatch(job_id=created.job_id, epoch=2))
    assert replacement is not None
    assert executor.execute(replacement)
    assert complete(database, replacement)
    assert fetcher.calls == 1


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
                query_variant="AI",
                since="2026-09-14T00:00:00Z",
                until="2026-09-15T00:00:00Z",
                idempotency_key="missing-evidence",
                policy_version="synthetic-policy-v1",
                retention_days=7,
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
        query_variant="AI",
        since="2026-09-14T00:00:00Z",
        until="2026-09-15T00:00:00Z",
        idempotency_key="replay-after-revocation",
        policy_version="synthetic-policy-v1",
        retention_days=7,
    )
    created = CollectionService(database, admitted, evidence_configured=True).create_run(data)

    replayed = CollectionService(database, RevokedSources()).create_run(data)
    assert replayed.id == created.id and replayed.job_id == created.job_id
    with pytest.raises(AppError, match="idempotency_conflict"):
        CollectionService(database, RevokedSources()).create_run(
            data.model_copy(update={"retention_days": 8})
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
        query_variant="AI",
        since="2026-09-14T00:00:00Z",
        until="2026-09-15T00:00:00Z",
        idempotency_key="concurrent-collection-run",
        policy_version="synthetic-policy-v1",
        retention_days=7,
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
            query_variant="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="revoked-before-fetch",
            policy_version="synthetic-policy-v1",
            retention_days=7,
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

    assert CollectionExecutor(collection, RevokedSources(), fetcher, MemoryStore()).execute(lease)
    assert complete(database, lease)
    persisted = collection.run(created.id)
    assert persisted.state == "failed" and persisted.stop_reason == "source_not_eligible"
    assert fetcher.calls == 0


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
    assert page.items[0].monitor_titles == ["AI 观察"]
    assert page.next_cursor is None


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
            query_variant="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="preflight-reject",
            policy_version="synthetic-policy-v1",
            retention_days=7,
        )
    )
    lease = collection.claim_for_job(started.job_id, 1)
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
        collection.commit_page(page, store)

    assert store.objects == {}


def test_old_fence_cannot_commit_and_stale_commit_leaves_orphan_object(database):
    sources = AdmittedSources()
    monitors = MonitorService(database, sources)
    collection = CollectionService(database, sources, evidence_configured=True)
    active = monitor(monitors, "隔离", ["AI"])
    started = collection.create_run(
        CollectionRunInput(
            monitor_id=active.id,
            expected_version=active.current_version,
            source="bilibili",
            query_variant="AI",
            since="2026-09-14T00:00:00Z",
            until="2026-09-15T00:00:00Z",
            idempotency_key="old-fence",
            policy_version="synthetic-policy-v1",
            retention_days=7,
        )
    )
    lease = collection.claim_for_job(started.job_id, 1)
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
        collection.commit_page(page, store)
    assert len(store.objects) == 1
    with database() as session:
        assert session.scalar(select(func.count()).select_from(RawPage)) == 0
