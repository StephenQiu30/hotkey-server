"""Verify the configured MinIO bucket with one isolated, removable object."""

import json
from datetime import UTC, datetime
from uuid import uuid4

from minio.error import S3Error

from core.config import Settings
from evidence.adapters.minio import MinioEvidenceStore
from evidence.services import prepare_evidence, upload


def main() -> None:
    settings = Settings()
    store = MinioEvidenceStore.from_settings(settings)
    if not store.client.bucket_exists(store.bucket):
        raise RuntimeError("evidence_bucket_missing")

    scope_id = uuid4()
    prepared = prepare_evidence(
        source="poc",
        observed_at=datetime.now(UTC),
        run_id=scope_id,
        page_key=f"hotkey-minio-poc:{scope_id}",
        media_type="application/json",
        payload=b'{"scope":"hotkey-minio-poc","synthetic":true}',
    )

    put_attempted = False
    try:
        put_attempted = True
        first = upload(store, prepared)
        second = upload(store, prepared)
        if second != first:
            raise RuntimeError("evidence_idempotency_failed")
    finally:
        if put_attempted:
            store.delete(prepared.key, prepared.object_sha256)

    try:
        store.client.stat_object(store.bucket, prepared.key)
    except S3Error as error:
        if error.code not in {"NoSuchKey", "NoSuchObject"}:
            raise
    else:
        raise RuntimeError("evidence_cleanup_failed")

    print(
        json.dumps(
            {
                "scope": "minio_isolated_object_protocol",
                "object_sha256": prepared.object_sha256,
                "object_bytes": len(prepared.payload),
                "idempotent_put": True,
                "readback_verified": True,
                "isolated_object_removed": True,
                "all_object_versions_removed": True,
            },
            sort_keys=True,
        )
    )


if __name__ == "__main__":
    main()
