from __future__ import annotations

import hashlib
import json
import os
import stat
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from io import BytesIO
from pathlib import Path
from uuid import uuid4

import pytest
from minio import Minio
from sqlalchemy import create_engine, text
from sqlalchemy.engine import make_url

from backups.adapters.minio import MinioObjectInventory
from backups.adapters.postgres import PostgresDumpAdapter
from backups.schemas import BackupManifest, EvidenceBackupMode, EvidenceObjectState
from backups.services import BackupService


@pytest.fixture
def backup_environment() -> Iterator[tuple[str, Minio, str, str]]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    endpoint = os.getenv("HOTKEY_TEST_MINIO_ENDPOINT")
    access_key = os.getenv("HOTKEY_TEST_MINIO_ACCESS_KEY")
    secret_key = os.getenv("HOTKEY_TEST_MINIO_SECRET_KEY")
    bucket = os.getenv("HOTKEY_TEST_MINIO_BUCKET")
    if not all((database_url, endpoint, access_key, secret_key, bucket)):
        pytest.skip("PostgreSQL and MinIO test settings are required for backup integration")

    minio = Minio(
        endpoint,
        access_key=access_key,
        secret_key=secret_key,
        secure=os.getenv("HOTKEY_TEST_MINIO_SECURE", "false").lower() == "true",
    )
    assert minio.bucket_exists(bucket)
    engine = create_engine(database_url)
    owner_id = uuid4()
    policy_id = uuid4()
    retention_id = uuid4()
    resource_id = uuid4()
    now = datetime(2026, 9, 21, 14, tzinfo=UTC)
    object_name = f"backup-tests/{uuid4()}/evidence.bin"
    content = b"hotkey-backup-evidence"
    with engine.begin() as connection:
        connection.execute(
            text(
                "TRUNCATE source_capability_evidence, source_connection_versions, "
                "source_connections, provenance_manifest_inputs, provenance_manifests, "
                "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                "evidence_retention_policies, source_access_policies, "
                "resource_budget_reservations, resource_budget_windows, "
                "resource_budget_policies, resource_usage_attempts, "
                "resource_component_policies, job_stage_attempts, processed_messages, "
                "job_attempts, outbox_messages, jobs, monitor_topic_versions, monitor_topics, "
                "identity_sessions, identity_users"
            )
        )
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:owner_id, 'backup-owner', 'test-only-hash', 1, :now, :now)"
            ),
            {"owner_id": owner_id, "now": now},
        )
        connection.execute(
            text(
                "INSERT INTO source_access_policies "
                "(id, owner_id, source_key, capability, status, enabled, access_basis, "
                "terms_reference, processing_purpose, component_name, component_version, "
                "component_license, field_purposes, reviewed_at, review_expires_at, "
                "policy_version, created_at, updated_at) VALUES "
                "(:policy_id, :owner_id, 'manual', 'search', 'approved', true, "
                "'manual_import', 'test-fixture', 'backup test', 'fixture', '1', "
                "'project-internal', '{\"external_id\": \"stable identity\"}'::jsonb, "
                ":now, :review_expires_at, 1, :now, :now)"
            ),
            {
                "policy_id": policy_id,
                "owner_id": owner_id,
                "now": now,
                "review_expires_at": now + timedelta(days=31),
            },
        )
        connection.execute(
            text(
                "INSERT INTO evidence_retention_policies "
                "(id, owner_id, source_policy_id, source_policy_version, data_class, "
                "requested_days, source_max_days, effective_days, policy_version, "
                "created_at, updated_at) VALUES "
                "(:retention_id, :owner_id, :policy_id, 1, 'raw', 30, 30, 30, 1, :now, :now)"
            ),
            {
                "retention_id": retention_id,
                "owner_id": owner_id,
                "policy_id": policy_id,
                "now": now,
            },
        )
        connection.execute(
            text(
                "INSERT INTO evidence_resources "
                "(id, owner_id, resource_type, resource_id, source_policy_id, "
                "source_policy_version, retention_policy_id, retention_policy_version, "
                "data_class, collected_at, expires_at, cleanup_targets, created_at) VALUES "
                "(:id, :owner_id, 'backup_fixture', :resource_id, :policy_id, 1, "
                ":retention_id, 1, 'raw', :now, :expires_at, CAST(:targets AS jsonb), :now)"
            ),
            {
                "id": uuid4(),
                "owner_id": owner_id,
                "resource_id": resource_id,
                "policy_id": policy_id,
                "retention_id": retention_id,
                "now": now,
                "expires_at": now + timedelta(days=30),
                "targets": json.dumps([{"kind": "minio_object", "reference": object_name}]),
            },
        )
    minio.put_object(bucket, object_name, BytesIO(content), len(content))
    try:
        yield database_url, minio, bucket, object_name
    finally:
        minio.remove_object(bucket, object_name)
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE source_capability_evidence, source_connection_versions, "
                    "source_connections, provenance_manifest_inputs, provenance_manifests, "
                    "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                    "evidence_retention_policies, source_access_policies, "
                    "resource_budget_reservations, resource_budget_windows, "
                    "resource_budget_policies, resource_usage_attempts, "
                    "resource_component_policies, job_stage_attempts, processed_messages, "
                    "job_attempts, outbox_messages, jobs, monitor_topic_versions, monitor_topics, "
                    "identity_sessions, identity_users"
                )
            )
        engine.dispose()


def test_candidate_backup_uses_real_snapshot_archive_and_minio_inventory(
    backup_environment: tuple[str, Minio, str, str],
    tmp_path: Path,
) -> None:
    database_url, minio, bucket, object_name = backup_environment
    engine = create_engine(database_url)
    service = BackupService(
        engine=engine,
        archive_writer=PostgresDumpAdapter(database_url),
        object_inspector=MinioObjectInventory(minio, bucket),
        evidence_bucket=bucket,
        schema_path=Path(__file__).resolve().parents[2] / "database" / "schema.sql",
    )
    try:
        result = service.create_candidate(tmp_path)
    finally:
        engine.dispose()

    manifest_path = result.directory / "manifest.json"
    archive_path = result.directory / "database.dump"
    manifest_text = manifest_path.read_text()
    manifest = BackupManifest.model_validate_json(manifest_text)
    table_counts = {item.name: item.row_count for item in manifest.database.tables}

    assert result.manifest == manifest
    assert manifest.evidence_mode is EvidenceBackupMode.INVENTORY_ONLY
    assert manifest.restore_verified is False
    assert manifest.secrets_included is False
    assert len(manifest.database.tables) == 21
    assert table_counts["identity_users"] == 1
    assert table_counts["evidence_resources"] == 1
    assert len(manifest.evidence_objects) == 1
    assert manifest.evidence_objects[0].object_name == object_name
    assert manifest.evidence_objects[0].state is EvidenceObjectState.PRESENT
    assert manifest.evidence_objects[0].size_bytes == len(b"hotkey-backup-evidence")
    assert manifest.database.archive_sha256 == hashlib.sha256(archive_path.read_bytes()).hexdigest()
    assert stat.S_IMODE(result.directory.stat().st_mode) == 0o700
    assert stat.S_IMODE(manifest_path.stat().st_mode) == 0o600
    assert stat.S_IMODE(archive_path.stat().st_mode) == 0o600
    assert not (result.directory / "evidence").exists()

    parsed_url = make_url(database_url)
    secrets = [
        parsed_url.password,
        os.environ["HOTKEY_TEST_MINIO_ACCESS_KEY"],
        os.environ["HOTKEY_TEST_MINIO_SECRET_KEY"],
    ]
    assert all(secret not in manifest_text for secret in secrets if secret)
