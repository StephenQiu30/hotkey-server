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
            request_value="AI",
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
    )
    assert duplicate.duplicate is True
    assert duplicate.followup_run_count == 0

    with database() as session:
        runs = list(session.scalars(select(CollectionRun).order_by(CollectionRun.created_at)))
        assert len(runs) == 2
        detail = runs[1]
        assert detail.parent_run_id == created.id
        assert detail.operation == "fetch_post"
        assert detail.request_value == "bvid:BV1BVFWeHEaV"
        assert session.scalar(select(func.count()).select_from(Job)) == 2
        assert session.scalar(select(func.count()).select_from(Outbox)) == 2
        detail_job_id = detail.job_id
    assert complete(database, lease)

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
    assert complete(database, detail_lease)

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
                trigger="scheduled",
                schedule_slot=slot,
            )
        )
        execution = collection.claim_for_job(parent.job_id, index)
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


def test_post_detail_expands_one_root_comment_and_reply_page_with_context_match(database):
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
    assert complete(database, search_lease)

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
    assert complete(database, detail_lease)

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
    assert CollectionExecutor(collection, sources, StaticFetcher(comments_page), store).execute(
        comments_lease
    )
    assert complete(database, comments_lease)

    with database() as session:
        replies = session.scalar(
            select(CollectionRun).where(CollectionRun.operation == "list_replies")
        )
        assert replies is not None
        assert replies.parent_run_id == comments.id
        assert replies.request_value == "aid:113/root:441"
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
    assert CollectionExecutor(collection, sources, StaticFetcher(replies_page), store).execute(
        replies_lease
    )
    assert complete(database, replies_lease)

    inbox = {item.external_id: item for item in ContentService(database).inbox(20, None).items}
    assert set(inbox) == {"video:113", "comment:441", "comment:442"}
    assert inbox["comment:441"].relation_status == "resolved"
    assert inbox["comment:441"].monitor_titles == ["评论上下文"]
    assert inbox["comment:442"].relation_status == "resolved"
    assert inbox["comment:442"].parent_external_id == "comment:441"
    assert inbox["comment:442"].monitor_titles == ["评论上下文"]
    comment_run = collection.run(comments.id)
    assert comment_run.outcome == "partial"
    assert comment_run.stop_reason == "page_limit"
    reply_run = collection.run(replies.id)
    assert reply_run.outcome == "partial"
    assert reply_run.stop_reason == "page_limit"
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 4
        assert session.scalar(select(func.count()).select_from(Job)) == 4
        assert session.scalar(select(func.count()).select_from(Outbox)) == 4
        assert session.scalar(select(func.count()).select_from(RawPage)) == 4
        assert session.scalar(select(func.count()).select_from(CollectionCheckpoint)) == 4


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
                "budget": {"daily_requests": 4, "content_purchase_cost": 0},
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
            )
        )

    parent = create("budget-parent")
    create("budget-competing-run")
    create("budget-second-competing-run")
    create("budget-third-competing-run")
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
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 4
        assert session.scalar(select(func.count()).select_from(Job)) == 4


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
    assert CollectionExecutor(collection, sources, fetcher, MemoryStore()).execute(lease)
    assert complete(database, lease)
    persisted = collection.run(created.id)
    assert persisted.state == "cancelled"
    assert persisted.outcome == "partial"
    assert persisted.stop_reason == "monitor_inactive"
    assert fetcher.calls == 0


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
        )
    )
    execution = collection.claim_for_job(created.job_id, 1)
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
    )
    assert committed.followup_run_count == 0
    persisted = collection.run(created.id)
    assert persisted.outcome == "partial"
    assert persisted.stop_reason == "detail_not_eligible"
    with database() as session:
        assert session.scalar(select(func.count()).select_from(CollectionRun)) == 1
        assert session.scalar(select(func.count()).select_from(Job)) == 1


def test_worker_crash_after_page_commit_completes_job_without_refetch(database):
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
                request_value="AI",
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
        request_value="AI",
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
        request_value="AI",
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
            request_value="AI",
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
            request_value="AI",
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
