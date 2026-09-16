import json

import pytest
from minio.error import S3Error
from minio.lifecycleconfig import (
    AbortIncompleteMultipartUpload,
    Expiration,
    LifecycleConfig,
    NoncurrentVersionExpiration,
    Rule,
)
from minio.objectlockconfig import DAYS, GOVERNANCE, ObjectLockConfig
from minio.versioningconfig import ENABLED, VersioningConfig
from urllib3.response import HTTPResponse

from evidence.audit import audit_bucket


def s3_error(code: str) -> S3Error:
    return S3Error(HTTPResponse(), code, "redacted", "resource", "request", "host", "bucket", "key")


class PolicyClient:
    def __init__(self, *, error_code: str | None = None, configured: bool = True):
        self.error_code = error_code
        self.configured = configured

    def bucket_exists(self, bucket: str) -> bool:
        assert bucket == "private-bucket-name"
        return True

    def get_bucket_versioning(self, bucket: str) -> VersioningConfig:
        if self.error_code:
            raise s3_error(self.error_code)
        return VersioningConfig(status=ENABLED if self.configured else None)

    def get_object_lock_config(self, bucket: str) -> ObjectLockConfig:
        if self.error_code:
            raise s3_error(self.error_code)
        if not self.configured:
            raise s3_error("ObjectLockConfigurationNotFoundError")
        return ObjectLockConfig(mode=GOVERNANCE, duration=30, duration_unit=DAYS)

    def get_bucket_lifecycle(self, bucket: str) -> LifecycleConfig | None:
        if self.error_code:
            raise s3_error(self.error_code)
        if not self.configured:
            return None
        return LifecycleConfig(
            [
                Rule(
                    "Enabled",
                    rule_id="private-rule-name",
                    abort_incomplete_multipart_upload=AbortIncompleteMultipartUpload(7),
                    expiration=Expiration(days=90),
                    noncurrent_version_expiration=NoncurrentVersionExpiration(30, 2),
                ),
                Rule("Disabled", expiration=Expiration(days=1)),
            ]
        )


def test_audit_summarizes_enabled_retention_controls_without_identifiers():
    result = audit_bucket(PolicyClient(), "private-bucket-name")

    assert result == {
        "scope": "existing_minio_retention_controls",
        "bucket_access": "reachable",
        "versioning": {"read": "ok", "status": "enabled"},
        "object_lock": {
            "read": "ok",
            "status": "enabled",
            "default_retention": {"mode": "governance", "duration": 30, "unit": "days"},
        },
        "lifecycle": {
            "read": "ok",
            "status": "configured",
            "enabled_rule_count": 1,
            "current_expiration_days": [90],
            "noncurrent_expiration_days": [30],
            "newer_noncurrent_versions": [2],
            "abort_incomplete_multipart_days": [7],
            "expired_delete_marker_rule_count": 0,
        },
    }
    rendered = json.dumps(result)
    assert "private-bucket-name" not in rendered
    assert "private-rule-name" not in rendered


def test_audit_distinguishes_absent_controls_from_denied_reads():
    absent = audit_bucket(PolicyClient(configured=False), "private-bucket-name")
    denied = audit_bucket(PolicyClient(error_code="AccessDenied"), "private-bucket-name")

    assert absent["versioning"] == {"read": "ok", "status": "unversioned"}
    assert absent["object_lock"] == {"read": "ok", "status": "absent"}
    assert absent["lifecycle"] == {"read": "ok", "status": "absent"}
    assert denied["versioning"] == {"read": "denied", "status": "unknown"}
    assert denied["object_lock"] == {"read": "denied", "status": "unknown"}
    assert denied["lifecycle"] == {"read": "denied", "status": "unknown"}


def test_audit_reports_bucket_access_denial_without_policy_calls():
    client = PolicyClient()

    def denied(_: str) -> bool:
        raise s3_error("AccessDenied")

    client.bucket_exists = denied  # type: ignore[method-assign]
    assert audit_bucket(client, "private-bucket-name") == {
        "scope": "existing_minio_retention_controls",
        "bucket_access": "denied",
        "versioning": {"read": "not_attempted", "status": "unknown"},
        "object_lock": {"read": "not_attempted", "status": "unknown"},
        "lifecycle": {"read": "not_attempted", "status": "unknown"},
    }


def test_audit_propagates_unknown_s3_failures():
    with pytest.raises(S3Error) as failure:
        audit_bucket(PolicyClient(error_code="InternalError"), "private-bucket-name")

    assert failure.value.code == "InternalError"
