from dataclasses import dataclass
from datetime import UTC, datetime
from hashlib import sha256
from typing import cast
from uuid import UUID, uuid4

from sqlalchemy import and_, func, or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from collection.models import CollectionBudgetUsage, CollectionCheckpoint, CollectionRun
from collection.schemas import (
    CollectionExecutionInput,
    CollectionRunInput,
    CollectionRunPage,
    CollectionRunView,
    PageCommitInput,
    PageCommitView,
)
from contents.services import upsert_content
from core.clock import utcnow
from core.errors import AppError
from evidence.contracts import EvidenceStore
from evidence.services import (
    PreparedEvidence,
    prepare_evidence,
    raw_page_identity,
    record_raw_page,
    upload,
)
from jobs.execution import enqueue
from monitors.services import (
    active_monitor_configuration,
    content_is_matched,
    match_content,
    monitor_query_spec,
    monitor_version_identity,
    monitor_version_is_active,
    query_match_reasons,
)
from sources.schemas import QueryPreviewInput, SourceName
from sources.services import DISCOVERY_REFERENCE_LIMIT, SourceService


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
            if usage.reserved_requests >= usage.limit_requests:
                raise AppError("request_budget_exhausted", 429)
            usage.reserved_requests += 1
            usage.updated_at = now
            run_id = uuid4()
            job = enqueue(
                session,
                "collection:" + sha256(data.idempotency_key.encode()).hexdigest(),
                kind="collect_page",
            )
            run = CollectionRun(
                id=run_id,
                job_id=job.id,
                parent_run_id=None,
                monitor_version_id=configuration.monitor_version_id,
                source=data.source,
                operation=data.operation,
                request_value=data.request_value,
                idempotency_key=data.idempotency_key,
                policy_version=data.policy_version,
                retention_days=data.retention_days,
                trigger=data.trigger,
                ingestion_mode=data.ingestion_mode,
                schedule_slot=data.schedule_slot,
                budget_day=budget_day,
                reserved_requests=1,
                state="queued",
                outcome=None,
                fencing_token=0,
                window_since=data.since,
                window_until=data.until,
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
            return CollectionRunView.model_validate(run)

    def run(self, identity: UUID) -> CollectionRunView:
        with self.factory() as session:
            run = session.get(CollectionRun, identity)
            if run is None:
                raise AppError("collection_run_not_found", 404)
            return CollectionRunView.model_validate(run)

    def has_scheduled_run(
        self,
        monitor_version_id: UUID,
        source: str,
        operation: str,
        request_value: str,
        schedule_slot: datetime,
    ) -> bool:
        with self.factory() as session:
            return (
                session.scalar(
                    select(CollectionRun.id).where(
                        CollectionRun.monitor_version_id == monitor_version_id,
                        CollectionRun.source == source,
                        CollectionRun.operation == operation,
                        CollectionRun.request_value == request_value,
                        CollectionRun.schedule_slot == schedule_slot,
                    )
                )
                is not None
            )

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

    def claim_for_job(self, job_id: UUID, fencing_token: int) -> CollectionExecutionInput | None:
        with self.factory.begin() as session:
            run = session.scalar(
                select(CollectionRun).where(CollectionRun.job_id == job_id).with_for_update()
            )
            if run is None or run.state not in {"queued", "running"}:
                return None
            if not monitor_version_is_active(session, run.monitor_version_id):
                run.state = "cancelled"
                run.outcome = "partial"
                run.stop_reason = "monitor_inactive"
                run.completed_at = utcnow()
                return None
            run.state = "running"
            run.fencing_token = fencing_token
            run.started_at = run.started_at or utcnow()
            return CollectionExecutionInput(
                run_id=run.id,
                job_id=run.job_id,
                parent_run_id=run.parent_run_id,
                fencing_token=run.fencing_token,
                source=run.source,
                operation=run.operation,
                request_value=run.request_value,
                since=run.window_since,
                until=run.window_until,
                policy_version=run.policy_version,
                retention_days=run.retention_days,
                ingestion_mode=run.ingestion_mode,
            )

    def result_committed_for_job(self, job_id: UUID) -> bool:
        with self.factory() as session:
            state = session.scalar(
                select(CollectionRun.state).where(CollectionRun.job_id == job_id)
            )
            return state in {"completed", "failed", "cancelled"}

    def fail_run(self, identity: UUID, fencing_token: int, reason: str) -> bool:
        with self.factory.begin() as session:
            run = session.scalar(
                select(CollectionRun).where(CollectionRun.id == identity).with_for_update()
            )
            if run is None or run.state != "running" or run.fencing_token != fencing_token:
                return False
            run.state = "failed"
            run.outcome = "failed"
            run.stop_reason = reason[:80]
            run.completed_at = utcnow()
            return True

    @staticmethod
    def _page_state(
        session: Session,
        data: PageCommitInput,
        prepared: PreparedEvidence,
        *,
        lock: bool,
    ) -> tuple[CollectionRun, CollectionCheckpoint | None]:
        query = select(CollectionRun).where(CollectionRun.id == data.run_id)
        if lock:
            query = query.with_for_update()
        run = session.scalar(query)
        if run is None:
            raise AppError("collection_run_not_found", 404)
        if run.fencing_token != data.fencing_token:
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
        created = 0
        for request_value in list(dict.fromkeys(references))[:DISCOVERY_REFERENCE_LIMIT]:
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

    def commit_page(self, data: PageCommitInput, store: EvidenceStore) -> PageCommitView:
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
            run, existing = self._page_state(session, data, prepared, lock=False)
            if existing is not None:
                return self._duplicate_view(run, existing)
        stored = upload(store, prepared)
        with self.factory.begin() as session:
            run, existing = self._page_state(session, data, prepared, lock=True)
            if existing is not None:
                return self._duplicate_view(run, existing)
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
            run.pages_count += 1
            run.items_count += len(data.result.items)
            run.bytes_count += data.result.response_bytes
            run.stop_reason = data.result.code
            followup_count, followup_stop = self._create_reference_runs(
                session,
                run,
                [reference.external_id for reference in data.result.references],
            )
            if data.result.cursor is None:
                run.state = "failed" if data.result.status == "failed" else "completed"
                run.outcome = "partial" if followup_stop else data.result.status
                run.stop_reason = followup_stop or run.stop_reason
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
