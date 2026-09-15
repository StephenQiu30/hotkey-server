import gzip
from dataclasses import dataclass
from datetime import datetime
from hashlib import sha256
from uuid import UUID, uuid4

from sqlalchemy.orm import Session

from core.errors import AppError
from evidence.contracts import EvidenceStore, StoredObject
from evidence.models import RawPage


@dataclass(frozen=True)
class PreparedEvidence:
    key: str
    payload: bytes
    payload_sha256: str
    object_sha256: str


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
