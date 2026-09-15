from hashlib import sha256
from uuid import UUID, uuid4

from sqlalchemy import func, select
from sqlalchemy.orm import Session, sessionmaker

from collection.models import CollectionCheckpoint, CollectionRun
from collection.schemas import (
    CollectionExecutionInput,
    CollectionRunInput,
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
    match_content,
    monitor_query_spec,
    monitor_version_identity,
    query_match_reasons,
)
from sources.schemas import QueryPreviewInput
from sources.services import SourceService


class CollectionService:
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
                    data.query_variant,
                    data.since,
                    data.until,
                    data.policy_version,
                    data.retention_days,
                )
                actual = (
                    existing.monitor_version_id,
                    existing.source,
                    existing.operation,
                    existing.query_variant,
                    existing.window_since,
                    existing.window_until,
                    existing.policy_version,
                    existing.retention_days,
                )
                if actual != expected:
                    raise AppError("idempotency_conflict", 409)
                return CollectionRunView.model_validate(existing)
            version_id, query_spec, source_ids = active_monitor_configuration(
                session, data.monitor_id, data.expected_version
            )
            if data.source not in source_ids:
                raise AppError("source_not_in_monitor", 409)
            if self.sources.activation_issues([data.source]):
                raise AppError("source_not_eligible", 409)
            if not self.evidence_configured:
                raise AppError("evidence_store_not_configured", 409)
            preview = self.sources.preview(
                QueryPreviewInput(
                    query_spec=query_spec,
                    source_ids=[data.source],
                    since=data.since,
                    until=data.until,
                )
            )
            if data.query_variant not in preview.sources[0].queries:
                raise AppError("query_variant_not_in_snapshot", 409)
            now = utcnow()
            run_id = uuid4()
            job = enqueue(
                session,
                "collection:" + sha256(data.idempotency_key.encode()).hexdigest(),
                kind="collect_page",
            )
            run = CollectionRun(
                id=run_id,
                job_id=job.id,
                monitor_version_id=version_id,
                source=data.source,
                operation=data.operation,
                query_variant=data.query_variant,
                idempotency_key=data.idempotency_key,
                policy_version=data.policy_version,
                retention_days=data.retention_days,
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

    def claim_for_job(self, job_id: UUID, fencing_token: int) -> CollectionExecutionInput | None:
        with self.factory.begin() as session:
            run = session.scalar(
                select(CollectionRun).where(CollectionRun.job_id == job_id).with_for_update()
            )
            if run is None or run.state not in {"queued", "running"}:
                return None
            run.state = "running"
            run.fencing_token = fencing_token
            run.started_at = run.started_at or utcnow()
            return CollectionExecutionInput(
                run_id=run.id,
                job_id=run.job_id,
                fencing_token=run.fencing_token,
                source=run.source,
                operation=run.operation,
                query_variant=run.query_variant,
                since=run.window_since,
                until=run.window_until,
                policy_version=run.policy_version,
                retention_days=run.retention_days,
            )

    def result_committed_for_job(self, job_id: UUID) -> bool:
        with self.factory() as session:
            state = session.scalar(
                select(CollectionRun.state).where(CollectionRun.job_id == job_id)
            )
            return state in {"completed", "failed"}

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
            duplicate=True,
        )

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
            if data.result.cursor is None:
                run.state = "failed" if data.result.status == "failed" else "completed"
                run.outcome = data.result.status
                run.completed_at = utcnow()
            session.flush()
            return PageCommitView(
                run_id=run.id,
                checkpoint_id=checkpoint.id,
                raw_page_id=raw_page.id,
                item_count=len(data.result.items),
                new_content_count=new_contents,
                new_version_count=new_versions,
                duplicate=False,
            )
