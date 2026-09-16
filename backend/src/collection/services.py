from dataclasses import dataclass
from datetime import UTC, date, datetime, timedelta
from hashlib import sha256
from typing import Literal, cast
from uuid import UUID, uuid4

from sqlalchemy import and_, func, or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from collection.models import CollectionBudgetUsage, CollectionCheckpoint, CollectionRun
from collection.schemas import (
    CollectionExecutionInput,
    CollectionRunBatchInput,
    CollectionRunBatchView,
    CollectionRunInput,
    CollectionRunPage,
    CollectionRunView,
    CommentTrackingView,
    PageCommitInput,
    PageCommitView,
)
from contents.services import (
    comment_tracking_target,
    lock_content_identities,
    page_contains_withdrawn_content,
    upsert_content,
)
from core.clock import utcnow
from core.errors import AppError
from evidence.contracts import EvidenceStore, StoredObject
from evidence.services import (
    PreparedEvidence,
    prepare_evidence,
    raw_page_identity,
    raw_page_run_ids,
    record_failed_upload_cleanup,
    record_raw_page,
    upload,
)
from jobs.contracts import Lease
from jobs.execution import (
    cancel_in_session,
    complete_in_session,
    continue_lease,
    enqueue,
    fail_lease,
    lease_has_status,
    lease_is_active,
    lease_was_continued,
    reschedule_lease,
)
from monitors.schemas import ActiveMonitorConfiguration
from monitors.services import (
    active_monitor_configuration,
    content_is_matched,
    lock_monitor_match_for_comment_tracking,
    mark_monitor_match_comment_tracking,
    match_content,
    monitor_query_spec,
    monitor_version_identity,
    monitor_version_is_active,
    query_match_reasons,
)
from sources.schemas import QueryPreviewInput, SourceName
from sources.services import FOLLOWUP_REFERENCE_LIMITS, SourceService


@dataclass(frozen=True)
class CollectionTrendRun:
    id: UUID
    source: str
    ingestion_mode: str
    policy_version: str
    state: str
    outcome: str | None
    window_since: datetime
    window_until: datetime


def collection_run_contexts(
    session: Session, identities: set[UUID]
) -> dict[UUID, CollectionTrendRun]:
    if not identities:
        return {}
    rows = session.scalars(select(CollectionRun).where(CollectionRun.id.in_(identities)))
    return {
        run.id: CollectionTrendRun(
            id=run.id,
            source=run.source,
            ingestion_mode=run.ingestion_mode,
            policy_version=run.policy_version,
            state=run.state,
            outcome=run.outcome,
            window_since=run.window_since,
            window_until=run.window_until,
        )
        for run in rows
    }


def collection_coverage_runs(
    session: Session,
    sources: set[str],
    since: datetime,
    until: datetime,
) -> list[CollectionTrendRun]:
    if not sources:
        return []
    rows = session.scalars(
        select(CollectionRun).where(
            CollectionRun.source.in_(sources),
            CollectionRun.window_since < until,
            CollectionRun.window_until > since,
        )
    )
    return [
        CollectionTrendRun(
            id=run.id,
            source=run.source,
            ingestion_mode=run.ingestion_mode,
            policy_version=run.policy_version,
            state=run.state,
            outcome=run.outcome,
            window_since=run.window_since,
            window_until=run.window_until,
        )
        for run in rows
    ]


class CollectionService:
    COMMENT_PAGE_LIMIT = 2
    FOLLOWUP_OPERATION = {
        "search_posts": "fetch_post",
        "fetch_post": "list_comments",
        "list_comments": "list_replies",
    }
    FOLLOWUP_STOP_REASONS = {
        "fetch_post": {
            "not_eligible": "detail_not_eligible",
            "invalid_reference": "invalid_detail_reference",
            "budget_exhausted": "detail_budget_exhausted",
        },
        "list_comments": {
            "not_eligible": "comments_not_eligible",
            "invalid_reference": "invalid_comment_reference",
            "budget_exhausted": "comment_budget_exhausted",
        },
        "list_replies": {
            "not_eligible": "replies_not_eligible",
            "invalid_reference": "invalid_reply_reference",
            "budget_exhausted": "reply_budget_exhausted",
        },
    }

    def __init__(
        self,
        factory: sessionmaker[Session],
        sources: SourceService | None = None,
        *,
        evidence_configured: bool = False,
    ):
        self.factory = factory
        self.sources = sources or SourceService()
        self.evidence_configured = evidence_configured

    @staticmethod
    def _batch_prefix(idempotency_key: str) -> str:
        return "batch:" + sha256(idempotency_key.encode()).hexdigest() + ":"

    @staticmethod
    def _batch_run_key(prefix: str, index: int, source: str, query: str) -> str:
        identity = sha256(f"{source}\0search_posts\0{query}".encode()).hexdigest()[:52]
        return f"{prefix}{index:03d}:{identity}"

    @staticmethod
    def _reserve_requests(
        session: Session,
        configuration: ActiveMonitorConfiguration,
        count: int,
        now: datetime,
    ) -> date:
        budget_day = now.date()
        session.execute(
            insert(CollectionBudgetUsage)
            .values(
                id=uuid4(),
                monitor_version_id=configuration.monitor_version_id,
                budget_day=budget_day,
                limit_requests=configuration.budget.daily_requests,
                reserved_requests=0,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(
                index_elements=[
                    CollectionBudgetUsage.monitor_version_id,
                    CollectionBudgetUsage.budget_day,
                ]
            )
        )
        usage = session.scalar(
            select(CollectionBudgetUsage)
            .where(
                CollectionBudgetUsage.monitor_version_id == configuration.monitor_version_id,
                CollectionBudgetUsage.budget_day == budget_day,
            )
            .with_for_update()
        )
        assert usage is not None
        if usage.limit_requests != configuration.budget.daily_requests:
            raise AppError("budget_snapshot_mismatch", 500)
        if usage.reserved_requests + count > usage.limit_requests:
            raise AppError("request_budget_exhausted", 429)
        usage.reserved_requests += count
        usage.updated_at = now
        return budget_day

    @staticmethod
    def _create_run_record(
        session: Session,
        *,
        configuration: ActiveMonitorConfiguration,
        source: SourceName,
        request_value: str,
        since: datetime,
        until: datetime,
        idempotency_key: str,
        policy_version: str,
        retention_days: int,
        trigger: Literal["manual", "scheduled"],
        ingestion_mode: Literal["live", "backfill"],
        schedule_slot: datetime | None,
        budget_day: date,
        now: datetime,
    ) -> CollectionRun:
        job = enqueue(
            session,
            "collection:" + sha256(idempotency_key.encode()).hexdigest(),
            kind="collect_page",
        )
        run = CollectionRun(
            id=uuid4(),
            job_id=job.id,
            parent_run_id=None,
            monitor_version_id=configuration.monitor_version_id,
            source=source,
            operation="search_posts",
            request_value=request_value,
            idempotency_key=idempotency_key,
            policy_version=policy_version,
            retention_days=retention_days,
            trigger=trigger,
            ingestion_mode=ingestion_mode,
            schedule_slot=schedule_slot,
            budget_day=budget_day,
            reserved_requests=1,
            state="queued",
            outcome=None,
            fencing_token=0,
            window_since=since,
            window_until=until,
            pages_count=0,
            items_count=0,
            bytes_count=0,
            stop_reason=None,
            created_at=now,
            started_at=None,
            completed_at=None,
        )
        session.add(run)
        session.flush()
        return run

    def create_monitor_runs(self, data: CollectionRunBatchInput) -> CollectionRunBatchView:
        prefix = self._batch_prefix(data.idempotency_key)
        with self.factory.begin() as session:
            lock_key = int.from_bytes(
                sha256(prefix.encode()).digest()[:8], byteorder="big", signed=True
            )
            session.execute(select(func.pg_advisory_xact_lock(lock_key)))
            existing = list(
                session.scalars(
                    select(CollectionRun)
                    .where(CollectionRun.idempotency_key.startswith(prefix))
                    .order_by(CollectionRun.idempotency_key)
                    .with_for_update()
                )
            )
            if existing:
                version_id = monitor_version_identity(
                    session, data.monitor_id, data.expected_version
                )
                stable_match = all(
                    run.monitor_version_id == version_id
                    and run.trigger == data.trigger
                    and run.ingestion_mode == data.ingestion_mode
                    and run.schedule_slot == data.schedule_slot
                    for run in existing
                )
                scheduled_window_matches = data.trigger == "manual" or all(
                    run.window_since == data.since and run.window_until == data.until
                    for run in existing
                )
                if not stable_match or not scheduled_window_matches:
                    raise AppError("idempotency_conflict", 409)
                return CollectionRunBatchView(
                    items=[CollectionRunView.model_validate(run) for run in existing],
                    replayed=True,
                )

            configuration = active_monitor_configuration(
                session, data.monitor_id, data.expected_version
            )
            if self.sources.activation_issues(configuration.source_ids):
                raise AppError("source_not_eligible", 409)
            if not self.evidence_configured:
                raise AppError("evidence_store_not_configured", 409)
            if data.trigger == "manual":
                until = utcnow()
                since = until - timedelta(minutes=configuration.schedule.interval_minutes)
                schedule_slot = None
            else:
                assert data.since is not None and data.until is not None
                since = data.since
                until = data.until
                schedule_slot = data.schedule_slot
            preview = self.sources.preview(
                QueryPreviewInput(
                    query_spec=configuration.query_spec,
                    source_ids=configuration.source_ids,
                    since=since,
                    until=until,
                )
            )
            entries = [
                (source.source, query) for source in preview.sources for query in source.queries
            ]
            if not entries:
                raise AppError("source_not_eligible", 409)
            now = utcnow()
            budget_day = self._reserve_requests(session, configuration, len(entries), now)
            policy_version = f"monitor-version-{configuration.version}"
            runs = [
                self._create_run_record(
                    session,
                    configuration=configuration,
                    source=source,
                    request_value=query,
                    since=since,
                    until=until,
                    idempotency_key=self._batch_run_key(prefix, index, source, query),
                    policy_version=policy_version,
                    retention_days=configuration.schedule.retention_days,
                    trigger=data.trigger,
                    ingestion_mode=data.ingestion_mode,
                    schedule_slot=schedule_slot,
                    budget_day=budget_day,
                    now=now,
                )
                for index, (source, query) in enumerate(entries)
            ]
            return CollectionRunBatchView(
                items=[CollectionRunView.model_validate(run) for run in runs],
                replayed=False,
            )

    def create_run(self, data: CollectionRunInput) -> CollectionRunView:
        with self.factory.begin() as session:
            lock_key = int.from_bytes(
                sha256(data.idempotency_key.encode()).digest()[:8],
                byteorder="big",
                signed=True,
            )
            session.execute(select(func.pg_advisory_xact_lock(lock_key)))
            existing = session.scalar(
                select(CollectionRun)
                .where(CollectionRun.idempotency_key == data.idempotency_key)
                .with_for_update()
            )
            if existing is not None:
                version_id = monitor_version_identity(
                    session, data.monitor_id, data.expected_version
                )
                expected = (
                    version_id,
                    data.source,
                    data.operation,
                    data.request_value,
                    data.since,
                    data.until,
                    data.policy_version,
                    data.retention_days,
                    data.trigger,
                    data.ingestion_mode,
                    data.schedule_slot,
                )
                actual = (
                    existing.monitor_version_id,
                    existing.source,
                    existing.operation,
                    existing.request_value,
                    existing.window_since,
                    existing.window_until,
                    existing.policy_version,
                    existing.retention_days,
                    existing.trigger,
                    existing.ingestion_mode,
                    existing.schedule_slot,
                )
                if actual != expected:
                    raise AppError("idempotency_conflict", 409)
                return CollectionRunView.model_validate(existing)
            configuration = active_monitor_configuration(
                session, data.monitor_id, data.expected_version
            )
            if data.source not in configuration.source_ids:
                raise AppError("source_not_in_monitor", 409)
            if self.sources.activation_issues([data.source]):
                raise AppError("source_not_eligible", 409)
            if not self.evidence_configured:
                raise AppError("evidence_store_not_configured", 409)
            preview = self.sources.preview(
                QueryPreviewInput(
                    query_spec=configuration.query_spec,
                    source_ids=[data.source],
                    since=data.since,
                    until=data.until,
                )
            )
            if data.request_value not in preview.sources[0].queries:
                raise AppError("request_value_not_in_snapshot", 409)
            now = utcnow()
            budget_day = self._reserve_requests(session, configuration, 1, now)
            run = self._create_run_record(
                session,
                configuration=configuration,
                source=data.source,
                request_value=data.request_value,
                since=data.since,
                until=data.until,
                idempotency_key=data.idempotency_key,
                policy_version=data.policy_version,
                retention_days=data.retention_days,
                trigger=data.trigger,
                ingestion_mode=data.ingestion_mode,
                schedule_slot=data.schedule_slot,
                budget_day=budget_day,
                now=now,
            )
            return CollectionRunView.model_validate(run)

    def start_comment_tracking(self, identity: UUID) -> CommentTrackingView:
        with self.factory.begin() as session:
            context = lock_monitor_match_for_comment_tracking(session, identity)
            target = comment_tracking_target(session, context.content_id)
            if not self.evidence_configured:
                raise AppError("evidence_store_not_configured", 409)

            page_runs = raw_page_run_ids(session, set(target.raw_page_ids))
            run_positions: dict[UUID, int] = {}
            for position, raw_page_id in enumerate(target.raw_page_ids):
                run_id = page_runs.get(raw_page_id)
                if run_id is not None:
                    run_positions.setdefault(run_id, position)
            parents = list(
                session.scalars(
                    select(CollectionRun)
                    .where(
                        CollectionRun.id.in_(run_positions),
                        CollectionRun.monitor_version_id == context.monitor_version_id,
                        CollectionRun.source == target.source,
                        CollectionRun.operation.in_(("fetch_post", "search_posts")),
                    )
                    .with_for_update()
                )
            )
            parents.sort(
                key=lambda run: (
                    0 if run.operation == "fetch_post" else 1,
                    run_positions[run.id],
                )
            )
            if not parents:
                raise AppError("comment_tracking_origin_missing", 409)
            parent = parents[0]
            operation = self.FOLLOWUP_OPERATION[parent.operation]
            source = cast(SourceName, parent.source)
            if self.sources.activation_issues([source], operation):
                raise AppError("source_not_eligible", 409)
            if not self.sources.request_value_is_valid(source, operation, target.request_value):
                raise AppError("invalid_comment_tracking_reference", 409)

            run = session.scalar(
                select(CollectionRun)
                .where(
                    CollectionRun.parent_run_id == parent.id,
                    CollectionRun.operation == operation,
                    CollectionRun.request_value == target.request_value,
                )
                .with_for_update()
            )
            replayed = run is not None
            if run is None:
                usage = session.scalar(
                    select(CollectionBudgetUsage)
                    .where(
                        CollectionBudgetUsage.monitor_version_id == parent.monitor_version_id,
                        CollectionBudgetUsage.budget_day == parent.budget_day,
                    )
                    .with_for_update()
                )
                if usage is None:
                    raise AppError("collection_budget_missing", 500)
                if usage.reserved_requests >= usage.limit_requests:
                    raise AppError("request_budget_exhausted", 429)
                now = utcnow()
                key = (
                    "manual-followup:"
                    + sha256(f"{parent.id}|{operation}|{target.request_value}".encode()).hexdigest()
                )
                job = enqueue(
                    session,
                    "collection:" + sha256(key.encode()).hexdigest(),
                    "collect_page",
                )
                run = CollectionRun(
                    id=uuid4(),
                    job_id=job.id,
                    parent_run_id=parent.id,
                    monitor_version_id=parent.monitor_version_id,
                    source=parent.source,
                    operation=operation,
                    request_value=target.request_value,
                    idempotency_key=key,
                    policy_version=parent.policy_version,
                    retention_days=parent.retention_days,
                    trigger=parent.trigger,
                    ingestion_mode=parent.ingestion_mode,
                    schedule_slot=parent.schedule_slot,
                    budget_day=parent.budget_day,
                    reserved_requests=1,
                    state="queued",
                    outcome=None,
                    fencing_token=0,
                    window_since=parent.window_since,
                    window_until=parent.window_until,
                    pages_count=0,
                    items_count=0,
                    bytes_count=0,
                    stop_reason=None,
                    created_at=now,
                    started_at=None,
                    completed_at=None,
                )
                session.add(run)
                usage.reserved_requests += 1
                usage.updated_at = now
                session.flush()
            mark_monitor_match_comment_tracking(session, context)
            return CommentTrackingView(
                match_id=context.match_id,
                content_id=target.selected_content_id,
                root_content_id=target.root_content_id,
                review_state="following",
                run=CollectionRunView.model_validate(run),
                replayed=replayed,
            )

    def run(self, identity: UUID) -> CollectionRunView:
        with self.factory() as session:
            run = session.get(CollectionRun, identity)
            if run is None:
                raise AppError("collection_run_not_found", 404)
            return CollectionRunView.model_validate(run)

    @staticmethod
    def _decode_cursor(cursor: str) -> tuple[datetime, UUID]:
        try:
            timestamp, identity = cursor.rsplit("|", 1)
            created_at = datetime.fromisoformat(timestamp)
            if created_at.tzinfo is None:
                raise ValueError("cursor timestamp must include a timezone")
            return created_at.astimezone(UTC), UUID(identity)
        except (TypeError, ValueError) as error:
            raise AppError("invalid_cursor", 422) from error

    @staticmethod
    def _encode_cursor(run: CollectionRun) -> str:
        return f"{run.created_at.astimezone(UTC).isoformat()}|{run.id}"

    def runs(self, limit: int, cursor: str | None) -> CollectionRunPage:
        with self.factory() as session:
            query = select(CollectionRun).order_by(
                CollectionRun.created_at.desc(), CollectionRun.id.desc()
            )
            if cursor is not None:
                created_at, identity = self._decode_cursor(cursor)
                query = query.where(
                    or_(
                        CollectionRun.created_at < created_at,
                        and_(
                            CollectionRun.created_at == created_at,
                            CollectionRun.id < identity,
                        ),
                    )
                )
            rows = list(session.scalars(query.limit(limit + 1)))
            return CollectionRunPage(
                items=[CollectionRunView.model_validate(row) for row in rows[:limit]],
                next_cursor=self._encode_cursor(rows[limit - 1]) if len(rows) > limit else None,
            )

    def claim_for_job(self, lease: Lease) -> CollectionExecutionInput | None:
        with self.factory.begin() as session:
            run = session.scalar(
                select(CollectionRun).where(CollectionRun.job_id == lease.job_id).with_for_update()
            )
            if run is None or run.job_id != lease.job_id:
                return None
            if not lease_is_active(session, lease):
                return None
            if run.state not in {"queued", "running"}:
                return None
            if not monitor_version_is_active(session, run.monitor_version_id):
                if cancel_in_session(session, lease.job_id):
                    run.state = "cancelled"
                    run.outcome = "partial"
                    run.stop_reason = "monitor_inactive"
                    run.completed_at = utcnow()
                return None
            run.state = "running"
            run.fencing_token = lease.fencing_token
            run.started_at = run.started_at or utcnow()
            checkpoint = session.scalar(
                select(CollectionCheckpoint)
                .where(CollectionCheckpoint.run_id == run.id)
                .order_by(
                    CollectionCheckpoint.committed_at.desc(),
                    CollectionCheckpoint.id.desc(),
                )
                .limit(1)
            )
            return CollectionExecutionInput(
                run_id=run.id,
                job_id=run.job_id,
                parent_run_id=run.parent_run_id,
                fencing_token=lease.fencing_token,
                source=run.source,
                operation=run.operation,
                request_value=run.request_value,
                cursor=checkpoint.cursor if checkpoint is not None else None,
                since=run.window_since,
                until=run.window_until,
                policy_version=run.policy_version,
                retention_days=run.retention_days,
                ingestion_mode=run.ingestion_mode,
            )

    def cancel_job(self, job_id: UUID) -> bool:
        with self.factory.begin() as session:
            run = session.scalar(
                select(CollectionRun).where(CollectionRun.job_id == job_id).with_for_update()
            )
            if run is None or run.state not in {"queued", "running"}:
                return False
            if not cancel_in_session(session, job_id):
                return False
            run.state = "cancelled"
            run.outcome = "partial"
            run.stop_reason = "user_cancelled"
            run.completed_at = utcnow()
            return True

    @staticmethod
    def _running_run_for_lease(
        session: Session,
        identity: UUID,
        lease: Lease,
    ) -> CollectionRun | None:
        run = session.scalar(
            select(CollectionRun).where(CollectionRun.id == identity).with_for_update()
        )
        if (
            run is None
            or run.job_id != lease.job_id
            or run.state != "running"
            or run.fencing_token != lease.fencing_token
        ):
            return None
        return run

    @staticmethod
    def _mark_failed(run: CollectionRun, reason: str) -> None:
        run.state = "failed"
        run.outcome = "failed"
        run.stop_reason = reason[:80]
        run.completed_at = utcnow()

    def fail_run(self, lease: Lease, identity: UUID, reason: str) -> bool:
        with self.factory.begin() as session:
            run = self._running_run_for_lease(session, identity, lease)
            if run is None or not fail_lease(session, lease):
                return False
            self._mark_failed(run, reason)
            return True

    def defer_run(
        self,
        lease: Lease,
        identity: UUID,
        reason: str,
        delay_seconds: int,
    ) -> bool:
        with self.factory.begin() as session:
            run = self._running_run_for_lease(session, identity, lease)
            if run is None:
                return False
            usage = session.scalar(
                select(CollectionBudgetUsage)
                .where(
                    CollectionBudgetUsage.monitor_version_id == run.monitor_version_id,
                    CollectionBudgetUsage.budget_day == run.budget_day,
                )
                .with_for_update()
            )
            if usage is None:
                raise AppError("collection_budget_missing", 500)
            if usage.reserved_requests >= usage.limit_requests:
                if not fail_lease(session, lease):
                    return False
                self._mark_failed(run, "retry_budget_exhausted")
                return True
            disposition = reschedule_lease(session, lease, delay_seconds)
            if disposition == "stale":
                return False
            if disposition == "exhausted":
                self._mark_failed(run, f"{reason}_retry_exhausted")
                return True
            usage.reserved_requests += 1
            usage.updated_at = utcnow()
            run.reserved_requests += 1
            run.state = "queued"
            run.stop_reason = reason[:80]
            run.completed_at = None
            return True

    @staticmethod
    def _page_state(
        session: Session,
        data: PageCommitInput,
        prepared: PreparedEvidence,
        lease: Lease,
        *,
        lock: bool,
    ) -> tuple[CollectionRun, CollectionCheckpoint | None]:
        query = select(CollectionRun).where(CollectionRun.id == data.run_id)
        if lock:
            query = query.with_for_update()
        run = session.scalar(query)
        if run is None:
            raise AppError("collection_run_not_found", 404)
        if (
            lease.kind != "collect_page"
            or lease.job_id != run.job_id
            or lease.fencing_token != data.fencing_token
            or run.fencing_token != lease.fencing_token
        ):
            raise AppError("stale_collection_lease", 409)
        if run.source != data.result.source or run.operation != data.result.operation:
            raise AppError("collection_result_mismatch", 409)
        if run.policy_version != data.policy_version:
            raise AppError("collection_policy_changed", 409)
        existing = session.scalar(
            select(CollectionCheckpoint).where(
                CollectionCheckpoint.run_id == run.id,
                CollectionCheckpoint.page_key == data.page_key,
            )
        )
        if existing is not None:
            fingerprint, payload_hash = raw_page_identity(session, existing.raw_page_id)
            if fingerprint != data.request_fingerprint or payload_hash != prepared.payload_sha256:
                raise AppError("page_commit_conflict", 409)
            return run, existing
        if run.state != "running":
            raise AppError("collection_run_not_running", 409)
        return run, None

    @staticmethod
    def _duplicate_view(run: CollectionRun, checkpoint: CollectionCheckpoint) -> PageCommitView:
        return PageCommitView(
            run_id=run.id,
            checkpoint_id=checkpoint.id,
            raw_page_id=checkpoint.raw_page_id,
            item_count=checkpoint.item_count,
            new_content_count=0,
            new_version_count=0,
            followup_run_count=0,
            duplicate=True,
        )

    def _create_reference_runs(
        self,
        session: Session,
        parent: CollectionRun,
        references: list[str],
    ) -> tuple[int, str | None]:
        operation = self.FOLLOWUP_OPERATION.get(parent.operation)
        if operation is None or not references:
            return 0, None
        if not monitor_version_is_active(session, parent.monitor_version_id):
            return 0, "monitor_inactive"
        stop_reasons = self.FOLLOWUP_STOP_REASONS[operation]
        if self.sources.activation_issues([cast(SourceName, parent.source)], operation):
            return 0, stop_reasons["not_eligible"]
        usage = session.scalar(
            select(CollectionBudgetUsage)
            .where(
                CollectionBudgetUsage.monitor_version_id == parent.monitor_version_id,
                CollectionBudgetUsage.budget_day == parent.budget_day,
            )
            .with_for_update()
        )
        if usage is None:
            raise AppError("collection_budget_missing", 500)
        existing_values = set(
            session.scalars(
                select(CollectionRun.request_value)
                .where(
                    CollectionRun.parent_run_id == parent.id,
                    CollectionRun.operation == operation,
                )
                .with_for_update()
            )
        )
        remaining = max(FOLLOWUP_REFERENCE_LIMITS[operation] - len(existing_values), 0)
        if remaining == 0:
            return 0, None
        candidates = [value for value in dict.fromkeys(references) if value not in existing_values][
            :remaining
        ]
        created = 0
        for request_value in candidates:
            if not self.sources.request_value_is_valid(
                cast(SourceName, parent.source), operation, request_value
            ):
                return created, stop_reasons["invalid_reference"]
            if usage.reserved_requests >= usage.limit_requests:
                return created, stop_reasons["budget_exhausted"]
            child_id = uuid4()
            key = (
                "followup:"
                + sha256(f"{parent.id}|{operation}|{request_value}".encode()).hexdigest()
            )
            job = enqueue(session, "collection:" + sha256(key.encode()).hexdigest(), "collect_page")
            session.add(
                CollectionRun(
                    id=child_id,
                    job_id=job.id,
                    parent_run_id=parent.id,
                    monitor_version_id=parent.monitor_version_id,
                    source=parent.source,
                    operation=operation,
                    request_value=request_value,
                    idempotency_key=key,
                    policy_version=parent.policy_version,
                    retention_days=parent.retention_days,
                    trigger=parent.trigger,
                    ingestion_mode=parent.ingestion_mode,
                    schedule_slot=parent.schedule_slot,
                    budget_day=parent.budget_day,
                    reserved_requests=1,
                    state="queued",
                    outcome=None,
                    fencing_token=0,
                    window_since=parent.window_since,
                    window_until=parent.window_until,
                    pages_count=0,
                    items_count=0,
                    bytes_count=0,
                    stop_reason=None,
                    created_at=utcnow(),
                    started_at=None,
                    completed_at=None,
                )
            )
            usage.reserved_requests += 1
            usage.updated_at = utcnow()
            created += 1
        return created, None

    def commit_page(
        self, data: PageCommitInput, store: EvidenceStore, lease: Lease
    ) -> PageCommitView:
        if data.result.response_sha256 is None:
            raise AppError("evidence_hash_missing", 409)
        if data.result.response_sha256 != sha256(data.payload).hexdigest():
            raise AppError("evidence_payload_mismatch", 409)
        if data.result.response_bytes != len(data.payload):
            raise AppError("evidence_size_mismatch", 409)
        prepared = prepare_evidence(
            data.result.source,
            data.result.observed_at,
            data.run_id,
            data.page_key,
            data.media_type,
            data.payload,
        )
        with self.factory() as session:
            run, existing = self._page_state(session, data, prepared, lease, lock=False)
            if existing is not None:
                with self.factory.begin() as settling_session:
                    settled_run, settled_checkpoint = self._page_state(
                        settling_session, data, prepared, lease, lock=True
                    )
                    assert settled_checkpoint is not None
                    self._validate_or_settle_run_lease(settling_session, settled_run, lease)
                    return self._duplicate_view(settled_run, settled_checkpoint)
            if page_contains_withdrawn_content(session, run.source, data.result.items):
                raise AppError("content_withdrawn", 409)
            if not lease_is_active(session, lease):
                raise AppError("stale_collection_lease", 409)
        stored = upload(store, prepared)
        try:
            with self.factory.begin() as session:
                run, existing = self._page_state(session, data, prepared, lease, lock=True)
                if existing is not None:
                    self._validate_or_settle_run_lease(session, run, lease)
                    return self._duplicate_view(run, existing)
                lock_content_identities(
                    session,
                    run.source,
                    {(item.provider_namespace, item.external_id) for item in data.result.items},
                )
                if page_contains_withdrawn_content(session, run.source, data.result.items):
                    raise AppError("content_withdrawn", 409)
                view = self._commit_uploaded_page(session, run, data, prepared, stored, lease)
                self._validate_or_settle_run_lease(session, run, lease)
                return view
        except AppError as error:
            try:
                store.delete(stored.key, stored.sha256)
            except Exception:
                with self.factory.begin() as session:
                    cleanup_run = session.get(CollectionRun, data.run_id)
                    if cleanup_run is None:
                        raise AppError("collection_run_not_found", 404) from error
                    record_failed_upload_cleanup(
                        session,
                        run_id=cleanup_run.id,
                        source=cleanup_run.source,
                        operation=cleanup_run.operation,
                        request_fingerprint=data.request_fingerprint,
                        media_type=data.media_type,
                        observed_at=data.result.observed_at,
                        retention_until=data.retention_until,
                        policy_version=data.policy_version,
                        response_bytes=data.result.response_bytes,
                        prepared=prepared,
                        stored=stored,
                    )
                cleanup_code = (
                    "content_withdrawn_evidence_cleanup_failed"
                    if error.code == "content_withdrawn"
                    else "evidence_rejection_cleanup_failed"
                )
                raise AppError(cleanup_code, 503) from error
            raise

    @staticmethod
    def _validate_or_settle_run_lease(session: Session, run: CollectionRun, lease: Lease) -> None:
        if run.state == "completed":
            settled = complete_in_session(session, lease) or lease_has_status(
                session, lease, "succeeded"
            )
        elif run.state == "failed":
            settled = fail_lease(session, lease) or lease_has_status(session, lease, "failed")
        elif run.state == "running":
            settled = lease_is_active(session, lease)
        elif run.state == "queued":
            settled = lease_was_continued(session, lease)
        else:
            settled = False
        if not settled:
            raise AppError("stale_collection_lease", 409)

    def _commit_uploaded_page(
        self,
        session: Session,
        run: CollectionRun,
        data: PageCommitInput,
        prepared: PreparedEvidence,
        stored: StoredObject,
        lease: Lease,
    ) -> PageCommitView:
        raw_page = record_raw_page(
            session,
            run_id=run.id,
            source=run.source,
            operation=run.operation,
            request_fingerprint=data.request_fingerprint,
            media_type=data.media_type,
            observed_at=data.result.observed_at,
            retention_until=data.retention_until,
            policy_version=data.policy_version,
            response_bytes=data.result.response_bytes,
            prepared=prepared,
            stored=stored,
        )
        session.flush()
        query_spec = monitor_query_spec(session, run.monitor_version_id)
        new_contents = 0
        new_versions = 0
        for item in data.result.items:
            written = upsert_content(
                session, item, run.source, raw_page.id, data.result.observed_at
            )
            new_contents += int(written.new_content)
            new_versions += int(written.new_version)
            reasons = query_match_reasons(query_spec, item.text)
            if (
                not reasons
                and written.root_content_id is not None
                and content_is_matched(session, run.monitor_version_id, written.root_content_id)
            ):
                reasons = ["root_context"]
            if reasons:
                match_content(
                    session,
                    run.monitor_version_id,
                    written.content_id,
                    reasons,
                    data.result.observed_at,
                )
        checkpoint = CollectionCheckpoint(
            id=uuid4(),
            run_id=run.id,
            page_key=data.page_key,
            cursor=data.result.cursor,
            raw_page_id=raw_page.id,
            item_count=len(data.result.items),
            committed_at=utcnow(),
        )
        session.add(checkpoint)
        prior_followup_stop = (
            run.stop_reason
            if run.stop_reason
            in {
                reason
                for reasons in self.FOLLOWUP_STOP_REASONS.values()
                for reason in reasons.values()
            }
            else None
        )
        run.pages_count += 1
        run.items_count += len(data.result.items)
        run.bytes_count += data.result.response_bytes
        run.stop_reason = data.result.code
        continuation = False
        page_budget_exhausted = False
        paged_operation = run.operation in {"list_comments", "list_replies"}
        if (
            paged_operation
            and data.result.status != "failed"
            and data.result.cursor is not None
            and run.pages_count < self.COMMENT_PAGE_LIMIT
        ):
            usage = session.scalar(
                select(CollectionBudgetUsage)
                .where(
                    CollectionBudgetUsage.monitor_version_id == run.monitor_version_id,
                    CollectionBudgetUsage.budget_day == run.budget_day,
                )
                .with_for_update()
            )
            if usage is None:
                raise AppError("collection_budget_missing", 500)
            if usage.reserved_requests >= usage.limit_requests:
                page_budget_exhausted = True
            else:
                disposition = continue_lease(session, lease)
                if disposition == "stale":
                    raise AppError("stale_collection_lease", 409)
                if disposition == "exhausted":
                    self._mark_failed(run, "page_attempts_exhausted")
                else:
                    usage.reserved_requests += 1
                    usage.updated_at = utcnow()
                    run.reserved_requests += 1
                    run.state = "queued"
                    run.completed_at = None
                    continuation = True
        if run.state == "failed" or data.result.status == "failed":
            followup_count, followup_stop = 0, None
        else:
            followup_count, followup_stop = self._create_reference_runs(
                session,
                run,
                [reference.external_id for reference in data.result.references],
            )
        coverage_stop = prior_followup_stop or followup_stop
        if page_budget_exhausted:
            run.state = "completed"
            run.outcome = "partial"
            run.stop_reason = "page_budget_exhausted"
            run.completed_at = utcnow()
        elif run.state == "failed":
            pass
        elif continuation:
            run.stop_reason = coverage_stop
        else:
            if data.result.status == "failed":
                run.state = "failed"
                run.outcome = "failed"
            else:
                run.state = "completed"
                page_limited = paged_operation and data.result.cursor is not None
                run.outcome = "partial" if coverage_stop or page_limited else data.result.status
                run.stop_reason = coverage_stop or (
                    "page_limit" if page_limited else run.stop_reason
                )
            run.completed_at = utcnow()
        session.flush()
        return PageCommitView(
            run_id=run.id,
            checkpoint_id=checkpoint.id,
            raw_page_id=raw_page.id,
            item_count=len(data.result.items),
            new_content_count=new_contents,
            new_version_count=new_versions,
            followup_run_count=followup_count,
            duplicate=False,
        )
