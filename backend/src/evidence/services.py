import gzip
from dataclasses import dataclass
from datetime import datetime
from hashlib import sha256
from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from contents.services import redact_deleted_content_for_raw_page
from core.clock import utcnow
from core.errors import AppError
from evidence.contracts import EvidenceStore, StoredObject
from evidence.models import RawPage


@dataclass(frozen=True)
class PreparedEvidence:
    key: str
    payload: bytes
    payload_sha256: str
    object_sha256: str


@dataclass(frozen=True)
class EvidenceCleanupResult:
    deleted: int
    failed: int


def raw_page_run_ids(session: Session, identities: set[UUID]) -> dict[UUID, UUID]:
    if not identities:
        return {}
    rows = session.execute(
        select(RawPage.id, RawPage.run_id).where(
            RawPage.id.in_(identities), RawPage.object_state == "available"
        )
    )
    return {identity: run_id for identity, run_id in rows}


def raw_page_identity(session: Session, identity: UUID) -> tuple[str, str]:
    raw_page = session.get(RawPage, identity)
    if raw_page is None:
        raise AppError("raw_page_missing", 500)
    return raw_page.request_fingerprint, raw_page.payload_sha256


def prepare_evidence(
    source: str,
    observed_at: datetime,
    run_id: UUID,
    page_key: str,
    media_type: str,
    payload: bytes,
) -> PreparedEvidence:
    payload_hash = sha256(payload).hexdigest()
    page_hash = sha256(page_key.encode()).hexdigest()[:32]
    extension = "json" if media_type == "application/json" else "html"
    key = f"raw/{source}/{observed_at:%Y/%m/%d}/{run_id}/{page_hash}/{payload_hash}.{extension}.gz"
    compressed = gzip.compress(payload, mtime=0)
    return PreparedEvidence(
        key=key,
        payload=compressed,
        payload_sha256=payload_hash,
        object_sha256=sha256(compressed).hexdigest(),
    )


def upload(store: EvidenceStore, prepared: PreparedEvidence) -> StoredObject:
    stored = store.put(prepared.key, prepared.payload, prepared.object_sha256)
    if (
        stored.bucket != store.bucket
        or stored.key != prepared.key
        or stored.sha256 != prepared.object_sha256
        or stored.size != len(prepared.payload)
    ):
        raise AppError("evidence_integrity_failed", 503)
    return stored


def record_raw_page(
    session: Session,
    *,
    run_id: UUID,
    source: str,
    operation: str,
    request_fingerprint: str,
    media_type: str,
    observed_at: datetime,
    retention_until: datetime,
    policy_version: str,
    response_bytes: int,
    prepared: PreparedEvidence,
    stored: StoredObject,
) -> RawPage:
    raw_page = RawPage(
        id=uuid4(),
        run_id=run_id,
        source=source,
        operation=operation,
        request_fingerprint=request_fingerprint,
        bucket=stored.bucket,
        object_key=stored.key,
        payload_sha256=prepared.payload_sha256,
        object_sha256=stored.sha256,
        response_bytes=response_bytes,
        object_bytes=stored.size,
        media_type=media_type,
        observed_at=observed_at,
        retention_until=retention_until,
        policy_version=policy_version,
    )
    session.add(raw_page)
    return raw_page


def record_failed_upload_cleanup(
    session: Session,
    *,
    run_id: UUID,
    source: str,
    operation: str,
    request_fingerprint: str,
    media_type: str,
    observed_at: datetime,
    retention_until: datetime,
    policy_version: str,
    response_bytes: int,
    prepared: PreparedEvidence,
    stored: StoredObject,
) -> RawPage:
    raw_page = session.scalar(
        select(RawPage).where(RawPage.object_key == stored.key).with_for_update()
    )
    created = raw_page is None
    if raw_page is None:
        raw_page = record_raw_page(
            session,
            run_id=run_id,
            source=source,
            operation=operation,
            request_fingerprint=request_fingerprint,
            media_type=media_type,
            observed_at=observed_at,
            retention_until=retention_until,
            policy_version=policy_version,
            response_bytes=response_bytes,
            prepared=prepared,
            stored=stored,
        )
    if raw_page.object_state != "deleted":
        raw_page.object_state = "failed"
        raw_page.cleanup_attempts = 1 if created else raw_page.cleanup_attempts + 1
        raw_page.cleanup_error_code = "evidence_delete_failed"
    return raw_page


def schedule_raw_page_deletions(session: Session, identities: tuple[UUID, ...]) -> int:
    if not identities:
        return 0
    rows = list(
        session.scalars(select(RawPage).where(RawPage.id.in_(identities)).with_for_update())
    )
    scheduled = 0
    for raw_page in rows:
        if raw_page.object_state == "deleted":
            continue
        raw_page.object_state = "delete_pending"
        raw_page.cleanup_error_code = None
        scheduled += 1
    return scheduled


class EvidenceDeletionService:
    def __init__(self, factory: sessionmaker[Session], store: EvidenceStore):
        self.factory = factory
        self.store = store

    def reconcile(self, limit: int) -> EvidenceCleanupResult:
        deleted = 0
        failed = 0
        processed: set[UUID] = set()
        for _ in range(limit):
            with self.factory() as session:
                statement = (
                    select(RawPage)
                    .where(RawPage.object_state.in_(("delete_pending", "failed")))
                    .order_by(RawPage.observed_at, RawPage.id)
                    .limit(1)
                )
                if processed:
                    statement = statement.where(RawPage.id.not_in(processed))
                raw_page = session.scalar(statement)
                if raw_page is None:
                    break
                identity = raw_page.id
                key = raw_page.object_key
                expected_sha256 = raw_page.object_sha256
                processed.add(identity)
            try:
                self.store.delete(key, expected_sha256)
            except Exception:
                with self.factory.begin() as session:
                    current = session.scalar(
                        select(RawPage).where(RawPage.id == identity).with_for_update()
                    )
                    if current is not None and current.object_state != "deleted":
                        current.object_state = "failed"
                        current.cleanup_attempts += 1
                        current.cleanup_error_code = "evidence_delete_failed"
                        audit(session, "evidence_delete_failed", str(identity))
                failed += 1
                continue
            with self.factory.begin() as session:
                current = session.scalar(
                    select(RawPage).where(RawPage.id == identity).with_for_update()
                )
                if current is None or current.object_state == "deleted":
                    continue
                current.object_state = "deleted"
                current.cleanup_attempts += 1
                current.cleanup_error_code = None
                current.deleted_at = utcnow()
                redacted = redact_deleted_content_for_raw_page(session, identity)
                audit(session, "evidence_deleted", f"{identity}:{redacted}")
                deleted += 1
        return EvidenceCleanupResult(deleted=deleted, failed=failed)
