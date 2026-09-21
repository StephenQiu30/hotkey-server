from __future__ import annotations

from minio import Minio
from minio.error import S3Error

from backups.schemas import EvidenceObjectMetadata

_MISSING_CODES = frozenset({"NoSuchKey", "NoSuchObject", "NoSuchVersion"})


class ObjectInventoryError(RuntimeError):
    """MinIO could not determine whether a referenced object exists."""


class MinioObjectInventory:
    def __init__(self, client: Minio, bucket: str) -> None:
        self._client = client
        self._bucket = bucket

    def stat(self, object_name: str) -> EvidenceObjectMetadata | None:
        try:
            item = self._client.stat_object(self._bucket, object_name)
        except S3Error as error:
            if error.code in _MISSING_CODES:
                return None
            raise ObjectInventoryError(
                f"MinIO object inventory failed with code {error.code}"
            ) from error
        if item.size is None or item.last_modified is None or item.etag is None:
            raise ObjectInventoryError("MinIO object metadata is incomplete")
        return EvidenceObjectMetadata(
            size_bytes=item.size,
            etag=item.etag.strip('"'),
            version_id=item.version_id,
            last_modified_at=item.last_modified,
        )
