from hashlib import sha256
from io import BytesIO

import urllib3
from minio import Minio
from minio.error import S3Error

from core.config import Settings
from evidence.contracts import StoredObject


class MinioEvidenceStore:
    def __init__(self, client: Minio, bucket: str):
        self.client = client
        self.bucket = bucket

    @classmethod
    def from_settings(cls, settings: Settings) -> "MinioEvidenceStore":
        if not settings.s3_configured:
            raise RuntimeError("evidence_store_not_configured")
        assert settings.s3_endpoint is not None
        assert settings.s3_access_key is not None
        assert settings.s3_secret_key is not None
        assert settings.s3_bucket is not None
        pool = urllib3.PoolManager(
            timeout=urllib3.Timeout(connect=5, read=30),
            retries=False,
        )
        return cls(
            Minio(
                settings.s3_endpoint,
                access_key=settings.s3_access_key.get_secret_value(),
                secret_key=settings.s3_secret_key.get_secret_value(),
                secure=settings.s3_secure,
                http_client=pool,
            ),
            settings.s3_bucket,
        )

    def _read(self, key: str, maximum: int) -> bytes:
        response = self.client.get_object(self.bucket, key)
        try:
            chunks: list[bytes] = []
            remaining = maximum + 1
            while remaining > 0:
                chunk = response.read(remaining)
                if not chunk:
                    break
                chunks.append(chunk)
                remaining -= len(chunk)
            return b"".join(chunks)
        finally:
            response.close()
            response.release_conn()

    def _existing_size(self, key: str) -> int | None:
        try:
            size = self.client.stat_object(self.bucket, key).size
            if size is None:
                raise RuntimeError("evidence_size_missing")
            return size
        except S3Error as error:
            if error.code in {"NoSuchKey", "NoSuchObject"}:
                return None
            raise

    def _verified(self, key: str, expected_sha256: str, expected_size: int) -> StoredObject:
        size = self._existing_size(key)
        if size != expected_size:
            raise RuntimeError("evidence_readback_mismatch")
        payload = self._read(key, expected_size)
        if len(payload) != expected_size or sha256(payload).hexdigest() != expected_sha256:
            raise RuntimeError("evidence_readback_mismatch")
        return StoredObject(
            bucket=self.bucket,
            key=key,
            sha256=expected_sha256,
            size=expected_size,
        )

    def put(self, key: str, payload: bytes, expected_sha256: str) -> StoredObject:
        if sha256(payload).hexdigest() != expected_sha256:
            raise ValueError("evidence_hash_mismatch")
        existing_size = self._existing_size(key)
        if existing_size is not None:
            if existing_size != len(payload):
                raise RuntimeError("evidence_object_conflict")
            existing = self._read(key, len(payload))
            if len(existing) != len(payload) or sha256(existing).hexdigest() != expected_sha256:
                raise RuntimeError("evidence_object_conflict")
            return StoredObject(self.bucket, key, expected_sha256, len(payload))
        self.client.put_object(
            self.bucket,
            key,
            BytesIO(payload),
            len(payload),
            content_type="application/gzip",
            metadata={"sha256": expected_sha256},
        )
        return self._verified(key, expected_sha256, len(payload))
