from typing import Protocol

from minio.error import S3Error
from minio.lifecycleconfig import LifecycleConfig
from minio.objectlockconfig import ObjectLockConfig
from minio.versioningconfig import VersioningConfig


class MinioPolicyReader(Protocol):
    def bucket_exists(self, bucket_name: str) -> bool: ...

    def get_bucket_versioning(self, bucket_name: str) -> VersioningConfig: ...

    def get_object_lock_config(self, bucket_name: str) -> ObjectLockConfig: ...

    def get_bucket_lifecycle(self, bucket_name: str) -> LifecycleConfig | None: ...


def _s3_state(error: S3Error, absent_codes: set[str] | None = None) -> str:
    if error.code in {"AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch"}:
        return "denied"
    if absent_codes and error.code in absent_codes:
        return "absent"
    raise error


def _not_attempted() -> dict[str, object]:
    return {"read": "not_attempted", "status": "unknown"}


def _audit_versioning(client: MinioPolicyReader, bucket: str) -> dict[str, object]:
    try:
        config = client.get_bucket_versioning(bucket)
    except S3Error as error:
        state = _s3_state(error)
        return {"read": state, "status": "unknown"}
    status = {
        "Enabled": "enabled",
        "Suspended": "suspended",
        None: "unversioned",
    }.get(config.status, "unknown")
    return {"read": "ok", "status": status}


def _audit_object_lock(client: MinioPolicyReader, bucket: str) -> dict[str, object]:
    try:
        config = client.get_object_lock_config(bucket)
    except S3Error as error:
        state = _s3_state(
            error,
            {
                "NoSuchObjectLockConfiguration",
                "ObjectLockConfigurationNotFound",
                "ObjectLockConfigurationNotFoundError",
            },
        )
        if state == "absent":
            return {"read": "ok", "status": "absent"}
        return {"read": state, "status": "unknown"}
    result: dict[str, object] = {"read": "ok", "status": "enabled"}
    if config.mode is not None and config.duration is not None and config.duration_unit is not None:
        result["default_retention"] = {
            "mode": config.mode.lower(),
            "duration": config.duration,
            "unit": config.duration_unit.lower(),
        }
    return result


def _integers(values: list[int | None]) -> list[int]:
    return sorted({value for value in values if value is not None})


def _audit_lifecycle(client: MinioPolicyReader, bucket: str) -> dict[str, object]:
    try:
        config = client.get_bucket_lifecycle(bucket)
    except S3Error as error:
        state = _s3_state(error, {"NoSuchLifecycleConfiguration"})
        if state == "absent":
            return {"read": "ok", "status": "absent"}
        return {"read": state, "status": "unknown"}
    if config is None:
        return {"read": "ok", "status": "absent"}
    enabled = [rule for rule in config.rules if rule.status == "Enabled"]
    expirations = [rule.expiration for rule in enabled if rule.expiration is not None]
    noncurrent = [
        rule.noncurrent_version_expiration
        for rule in enabled
        if rule.noncurrent_version_expiration is not None
    ]
    incomplete = [
        rule.abort_incomplete_multipart_upload
        for rule in enabled
        if rule.abort_incomplete_multipart_upload is not None
    ]
    return {
        "read": "ok",
        "status": "configured",
        "enabled_rule_count": len(enabled),
        "current_expiration_days": _integers([item.days for item in expirations]),
        "noncurrent_expiration_days": _integers([item.noncurrent_days for item in noncurrent]),
        "newer_noncurrent_versions": _integers(
            [item.newer_noncurrent_versions for item in noncurrent]
        ),
        "abort_incomplete_multipart_days": _integers(
            [item.days_after_initiation for item in incomplete]
        ),
        "expired_delete_marker_rule_count": sum(
            item.expired_object_delete_marker is True for item in expirations
        ),
    }


def audit_bucket(client: MinioPolicyReader, bucket: str) -> dict[str, object]:
    try:
        exists = client.bucket_exists(bucket)
    except S3Error as error:
        state = _s3_state(error)
        return {
            "scope": "existing_minio_retention_controls",
            "bucket_access": state,
            "versioning": _not_attempted(),
            "object_lock": _not_attempted(),
            "lifecycle": _not_attempted(),
        }
    if not exists:
        return {
            "scope": "existing_minio_retention_controls",
            "bucket_access": "missing",
            "versioning": _not_attempted(),
            "object_lock": _not_attempted(),
            "lifecycle": _not_attempted(),
        }
    return {
        "scope": "existing_minio_retention_controls",
        "bucket_access": "reachable",
        "versioning": _audit_versioning(client, bucket),
        "object_lock": _audit_object_lock(client, bucket),
        "lifecycle": _audit_lifecycle(client, bucket),
    }
