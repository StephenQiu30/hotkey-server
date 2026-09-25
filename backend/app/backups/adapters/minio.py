from __future__ import annotations

import hashlib
import os
from pathlib import Path
from uuid import uuid4

from minio import Minio
from minio.error import S3Error

from backups.schemas import (
    BackupManifest,
    EvidenceArchiveMetadata,
    EvidenceBackupMode,
    EvidenceObjectMetadata,
    EvidenceObjectState,
)

_MISSING_CODES = frozenset({"NoSuchKey", "NoSuchObject", "NoSuchVersion"})


class ObjectInventoryError(RuntimeError):
    """MinIO could not determine whether a referenced object exists."""


class ObjectArchiveError(RuntimeError):
    """MinIO content could not be archived or verified safely."""


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

    def archive(
        self,
        object_name: str,
        *,
        metadata: EvidenceObjectMetadata,
        target: Path,
    ) -> EvidenceArchiveMetadata:
        if any(character in metadata.etag for character in ('"', "\r", "\n")):
            raise ObjectArchiveError("MinIO object ETag is invalid")

        response = None
        created = False
        result: EvidenceArchiveMetadata | None = None
        operation_error: Exception | None = None
        try:
            response = self._client.get_object(
                self._bucket,
                object_name,
                version_id=metadata.version_id,
                request_headers={"If-Match": f'"{metadata.etag}"'},
            )
            descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            created = True
            digest = hashlib.sha256()
            size_bytes = 0
            with os.fdopen(descriptor, "wb") as output:
                while chunk := response.read(64 * 1024):
                    digest.update(chunk)
                    size_bytes += len(chunk)
                    output.write(chunk)
                output.flush()
                os.fsync(output.fileno())
            if size_bytes != metadata.size_bytes:
                raise ObjectArchiveError("MinIO object size changed during archive")
            result = EvidenceArchiveMetadata(
                size_bytes=size_bytes,
                sha256=digest.hexdigest(),
            )
        except Exception as error:
            operation_error = error
        finally:
            if response is not None:
                for cleanup in (response.close, response.release_conn):
                    try:
                        cleanup()
                    except Exception as error:
                        operation_error = operation_error or error

        if operation_error is not None:
            if created:
                target.unlink(missing_ok=True)
            if isinstance(operation_error, ObjectArchiveError):
                raise operation_error
            raise ObjectArchiveError("MinIO object content archive failed") from operation_error
        if result is None:
            raise ObjectArchiveError("MinIO object content archive produced no result")
        return result


class MinioEvidenceRestoreVerifier:
    def __init__(self, client: Minio, bucket: str) -> None:
        self._client = client
        self._bucket = bucket

    def verify(self, *, manifest: BackupManifest, candidate: Path) -> int:
        if manifest.evidence_mode is not EvidenceBackupMode.CONTENT_ARCHIVED:
            return 0

        attempt_id = uuid4().hex
        verified = 0
        for index, item in enumerate(manifest.evidence_objects):
            if item.state is not EvidenceObjectState.PRESENT:
                continue
            if item.archive_path is None or item.content_sha256 is None:
                raise ObjectArchiveError("MinIO evidence archive is incomplete")
            target = (
                f"hotkey-restore-verification/{manifest.backup_id.hex}/"
                f"{attempt_id}/{index:08d}.blob"
            )
            target_version: str | None = None
            upload_attempted = False
            operation_error: Exception | None = None
            try:
                upload_attempted = True
                uploaded = self._client.fput_object(
                    self._bucket,
                    target,
                    str(candidate / item.archive_path),
                )
                target_version = uploaded.version_id
                stored = self._client.stat_object(
                    self._bucket,
                    target,
                    version_id=target_version,
                )
                if stored.size != item.size_bytes:
                    raise ObjectArchiveError("MinIO restored object size does not match archive")
                response = self._client.get_object(
                    self._bucket,
                    target,
                    version_id=target_version,
                )
                try:
                    digest = hashlib.sha256()
                    size_bytes = 0
                    while chunk := response.read(64 * 1024):
                        digest.update(chunk)
                        size_bytes += len(chunk)
                finally:
                    for cleanup in (response.close, response.release_conn):
                        try:
                            cleanup()
                        except Exception as error:
                            operation_error = operation_error or error
                    if operation_error is not None:
                        raise ObjectArchiveError(
                            "MinIO restored object read failed"
                        ) from operation_error
                if size_bytes != item.size_bytes or digest.hexdigest() != item.content_sha256:
                    raise ObjectArchiveError("MinIO restored object content does not match archive")
                verified += 1
            except Exception as error:
                operation_error = error
            finally:
                if upload_attempted:
                    try:
                        self._remove_target(target, target_version)
                    except Exception as error:
                        operation_error = operation_error or error
                        raise ObjectArchiveError(
                            f"MinIO temporary restore cleanup failed: {target}"
                        ) from error
            if operation_error is not None:
                raise ObjectArchiveError(
                    f"MinIO evidence round-trip failed at {target}"
                ) from operation_error
        return verified

    def _remove_target(self, target: str, version_id: str | None) -> None:
        if version_id:
            self._client.remove_object(self._bucket, target, version_id=version_id)
            return
        try:
            current = self._client.stat_object(self._bucket, target)
        except S3Error as error:
            if error.code in _MISSING_CODES:
                return
            raise
        if current.version_id:
            self._client.remove_object(self._bucket, target, version_id=current.version_id)
        else:
            self._client.remove_object(self._bucket, target)
