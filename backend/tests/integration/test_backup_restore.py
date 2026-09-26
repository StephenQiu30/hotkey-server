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

from backups.adapters.minio import MinioEvidenceRestoreVerifier, MinioObjectInventory
from backups.adapters.postgres import BackupToolError, PostgresDumpAdapter
from backups.restore import BackupRestoreError, BackupRestoreService
from backups.schemas import (
    REQUIRED_BACKUP_SETTINGS,
    BackupDatabaseManifest,
    BackupManifest,
    BackupState,
    BackupTableCount,
    EvidenceBackupMode,
    EvidenceObjectInventory,
    EvidenceObjectState,
)
from backups.services import BackupError, BackupService, archive_evidence_objects


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
                "TRUNCATE collection_due_windows, hotlist_entries, hotlist_snapshots, "
                "content_version_relations, content_visibility_observations, "
                "content_observations, content_versions, "
                "content_discoveries, content_threads, content_records, "
                "source_capability_evidence, source_connection_versions, "
                "source_connections, provenance_manifest_inputs, provenance_manifests, "
                "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                "evidence_retention_policies, source_access_policies, "
                "resource_budget_reservations, resource_budget_windows, "
                "resource_budget_policies, resource_usage_attempts, "
                "resource_component_policies, job_stage_attempts, processed_messages, "
                "job_attempts, "
                "ai_calls, knowledge_exports, notification_deliveries, notification_targets, "
                "content_annotations, reports, monitor_schedules, "
                "outbox_messages, coverage_windows, "
                "jobs, followed_account_aliases, followed_accounts, "
                "monitor_topic_versions, monitor_topics, "
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
                "'project-internal', CAST(:field_purposes AS jsonb), "
                ":now, :review_expires_at, 1, :now, :now)"
            ),
            {
                "policy_id": policy_id,
                "owner_id": owner_id,
                "field_purposes": '{"external_id": "stable identity"}',
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
                    "TRUNCATE collection_due_windows, hotlist_entries, hotlist_snapshots, "
                    "content_version_relations, content_visibility_observations, "
                    "content_observations, content_versions, "
                    "content_discoveries, content_threads, content_records, "
                    "source_capability_evidence, source_connection_versions, "
                    "source_connections, provenance_manifest_inputs, provenance_manifests, "
                    "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                    "evidence_retention_policies, source_access_policies, "
                    "resource_budget_reservations, resource_budget_windows, "
                    "resource_budget_policies, resource_usage_attempts, "
                    "resource_component_policies, job_stage_attempts, processed_messages, "
                    "job_attempts, "
                    "ai_calls, knowledge_exports, notification_deliveries, notification_targets, "
                    "content_annotations, reports, monitor_schedules, "
                    "outbox_messages, coverage_windows, "
                    "jobs, followed_account_aliases, followed_accounts, "
                    "monitor_topic_versions, monitor_topics, "
                    "identity_sessions, identity_users"
                )
            )
        engine.dispose()


def test_minio_evidence_content_archive_and_restore_round_trip(tmp_path: Path) -> None:
    endpoint = os.getenv("HOTKEY_TEST_MINIO_ENDPOINT")
    access_key = os.getenv("HOTKEY_TEST_MINIO_ACCESS_KEY")
    secret_key = os.getenv("HOTKEY_TEST_MINIO_SECRET_KEY")
    bucket = os.getenv("HOTKEY_TEST_MINIO_BUCKET")
    if not all((endpoint, access_key, secret_key, bucket)):
        pytest.skip("MinIO test settings are required for evidence content verification")
    minio = Minio(
        endpoint,
        access_key=access_key,
        secret_key=secret_key,
        secure=os.getenv("HOTKEY_TEST_MINIO_SECURE", "false").lower() == "true",
    )
    assert minio.bucket_exists(bucket)
    object_store = MinioObjectInventory(minio, bucket)
    object_name = f"backup-tests/{uuid4()}/source.bin"
    content = b"hotkey isolated evidence content restore"
    upload = minio.put_object(bucket, object_name, BytesIO(content), len(content))
    now = datetime.now(UTC)
    resource_id = uuid4()
    try:
        metadata = object_store.stat(object_name)
        assert metadata is not None
        inventory = EvidenceObjectInventory(
            resource_record_id=resource_id,
            object_name=object_name,
            expires_at=now + timedelta(days=30),
            state=EvidenceObjectState.PRESENT,
            checked_at=now,
            size_bytes=metadata.size_bytes,
            etag=metadata.etag,
            version_id=metadata.version_id,
            last_modified_at=metadata.last_modified_at,
        )
        evidence_objects = archive_evidence_objects(
            (inventory,),
            archiver=object_store,
            directory=tmp_path / "evidence",
        )
        manifest = BackupManifest(
            format_version="hotkey.backup.v1",
            backup_id=uuid4(),
            state=BackupState.CANDIDATE,
            consistency_at=now - timedelta(seconds=1),
            created_at=now + timedelta(seconds=1),
            database=BackupDatabaseManifest(
                archive_path="database.dump",
                archive_sha256="a" * 64,
                archive_size_bytes=1,
                schema_sha256="b" * 64,
                server_version="18.4",
                pg_dump_version="pg_dump (PostgreSQL) 18.4",
                tables=(BackupTableCount(name="identity_users", row_count=0),),
            ),
            evidence_mode=EvidenceBackupMode.CONTENT_ARCHIVED,
            evidence_bucket=bucket,
            evidence_objects=evidence_objects,
            required_settings=REQUIRED_BACKUP_SETTINGS,
            secrets_included=False,
            restore_verified=False,
        )

        assert (tmp_path / evidence_objects[0].archive_path).read_bytes() == content
        assert (
            MinioEvidenceRestoreVerifier(minio, bucket).verify(
                manifest=manifest,
                candidate=tmp_path,
            )
            == 1
        )
        assert minio.stat_object(bucket, object_name).size == len(content)
    finally:
        if upload.version_id:
            minio.remove_object(bucket, object_name, version_id=upload.version_id)
        else:
            minio.remove_object(bucket, object_name)


def test_candidate_backup_uses_real_snapshot_archive_and_minio_inventory(
    backup_environment: tuple[str, Minio, str, str],
    tmp_path: Path,
) -> None:
    database_url, minio, bucket, object_name = backup_environment
    engine = create_engine(database_url)
    object_store = MinioObjectInventory(minio, bucket)
    service = BackupService(
        engine=engine,
        archive_writer=PostgresDumpAdapter(database_url),
        object_inspector=object_store,
        object_archiver=object_store,
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
    assert manifest.evidence_mode is EvidenceBackupMode.CONTENT_ARCHIVED
    assert manifest.restore_verified is False
    assert manifest.secrets_included is False
    assert set(table_counts) == {
        "ai_calls",
        "content_annotations",
        "coverage_windows",
        "content_discoveries",
        "content_observations",
        "content_records",
        "hotlist_snapshots",
        "hotlist_entries",
        "content_threads",
        "content_version_relations",
        "content_versions",
        "content_visibility_observations",
        "evidence_cleanup_targets",
        "evidence_deletions",
        "evidence_resources",
        "evidence_retention_policies",
        "followed_account_aliases",
        "followed_accounts",
        "identity_sessions",
        "identity_users",
        "job_attempts",
        "job_stage_attempts",
        "jobs",
        "knowledge_exports",
        "monitor_schedules",
        "monitor_topic_versions",
        "monitor_topics",
        "notification_deliveries",
        "notification_targets",
        "outbox_messages",
        "processed_messages",
        "provenance_manifest_inputs",
        "provenance_manifests",
        "resource_budget_policies",
        "resource_budget_reservations",
        "resource_budget_windows",
        "resource_component_policies",
        "resource_usage_attempts",
        "reports",
        "source_access_policies",
        "source_capability_evidence",
        "source_connection_versions",
        "source_connections",
    }
    assert table_counts["identity_users"] == 1
    assert table_counts["evidence_resources"] == 1
    assert len(manifest.evidence_objects) == 1
    assert manifest.evidence_objects[0].object_name == object_name
    assert manifest.evidence_objects[0].state is EvidenceObjectState.PRESENT
    assert manifest.evidence_objects[0].size_bytes == len(b"hotkey-backup-evidence")
    assert manifest.evidence_objects[0].archive_path is not None
    assert (
        manifest.evidence_objects[0].content_sha256
        == hashlib.sha256(b"hotkey-backup-evidence").hexdigest()
    )
    assert (
        result.directory / manifest.evidence_objects[0].archive_path
    ).read_bytes() == b"hotkey-backup-evidence"
    assert manifest.database.archive_sha256 == hashlib.sha256(archive_path.read_bytes()).hexdigest()
    assert stat.S_IMODE(result.directory.stat().st_mode) == 0o700
    assert stat.S_IMODE(manifest_path.stat().st_mode) == 0o600
    assert stat.S_IMODE(archive_path.stat().st_mode) == 0o600
    assert stat.S_IMODE((result.directory / "evidence").stat().st_mode) == 0o700
    assert (
        stat.S_IMODE((result.directory / manifest.evidence_objects[0].archive_path).stat().st_mode)
        == 0o600
    )

    parsed_url = make_url(database_url)
    secrets = [
        parsed_url.password,
        os.environ["HOTKEY_TEST_MINIO_ACCESS_KEY"],
        os.environ["HOTKEY_TEST_MINIO_SECRET_KEY"],
    ]
    assert all(secret not in manifest_text for secret in secrets if secret)


def test_restore_candidate_in_isolated_database_and_remove_it(
    backup_environment: tuple[str, Minio, str, str],
    tmp_path: Path,
) -> None:
    database_url, minio, bucket, _ = backup_environment
    engine = create_engine(database_url)
    object_store = MinioObjectInventory(minio, bucket)
    try:
        candidate = BackupService(
            engine=engine,
            archive_writer=PostgresDumpAdapter(database_url),
            object_inspector=object_store,
            object_archiver=object_store,
            evidence_bucket=bucket,
            schema_path=Path(__file__).resolve().parents[2] / "database" / "schema.sql",
        ).create_candidate(tmp_path)
        with engine.connect() as connection:
            before = {
                row[0]
                for row in connection.execute(
                    text("SELECT datname FROM pg_database WHERE datname LIKE 'hotkey_restore_%'")
                )
            }
        with pytest.raises(BackupRestoreError, match="another database"):
            BackupRestoreService(
                source_database_url=database_url,
                isolation_database_url=database_url,
                schema_path=Path(__file__).resolve().parents[2] / "database" / "schema.sql",
                evidence_restore_verifier=MinioEvidenceRestoreVerifier(minio, bucket),
            ).verify(candidate.directory)
        result = BackupRestoreService(
            source_database_url=database_url,
            isolation_database_url=make_url(database_url).set(database="postgres"),
            schema_path=Path(__file__).resolve().parents[2] / "database" / "schema.sql",
            evidence_restore_verifier=MinioEvidenceRestoreVerifier(minio, bucket),
        ).verify(candidate.directory)
        assert result.backup_id == candidate.manifest.backup_id
        assert result.table_count == len(candidate.manifest.database.tables)
        assert result.duration_seconds > 0
        assert result.database_restored is True
        assert result.evidence_objects_verified == 1
        with engine.connect() as connection:
            after = {
                row[0]
                for row in connection.execute(
                    text("SELECT datname FROM pg_database WHERE datname LIKE 'hotkey_restore_%'")
                )
            }
        assert after == before
        assert candidate.directory.exists()
        assert (
            BackupManifest.model_validate_json(
                (candidate.directory / "manifest.json").read_text()
            ).restore_verified
            is False
        )
    finally:
        engine.dispose()


def test_corrupt_candidate_rejected_without_changing_existing_backup(
    backup_environment: tuple[str, Minio, str, str],
    tmp_path: Path,
) -> None:
    database_url, minio, bucket, _ = backup_environment
    engine = create_engine(database_url)
    object_store = MinioObjectInventory(minio, bucket)
    try:
        candidate = BackupService(
            engine=engine,
            archive_writer=PostgresDumpAdapter(database_url),
            object_inspector=object_store,
            object_archiver=object_store,
            evidence_bucket=bucket,
            schema_path=Path(__file__).resolve().parents[2] / "database" / "schema.sql",
        ).create_candidate(tmp_path)
        original = (candidate.directory / "database.dump").read_bytes()
        damaged_root = tmp_path / "damaged"
        damaged_root.mkdir(mode=0o700)
        bad = damaged_root / candidate.directory.name
        bad.mkdir(mode=0o700)
        (bad / "manifest.json").write_bytes((candidate.directory / "manifest.json").read_bytes())
        (bad / "database.dump").write_bytes(original[:-1])
        (bad / "manifest.json").chmod(0o600)
        (bad / "database.dump").chmod(0o600)
        with pytest.raises(BackupRestoreError, match="candidate"):
            BackupRestoreService(
                source_database_url=database_url,
                isolation_database_url=make_url(database_url).set(database="postgres"),
                schema_path=Path(__file__).resolve().parents[2] / "database" / "schema.sql",
            ).verify(bad)
        assert (candidate.directory / "database.dump").read_bytes() == original
        assert (candidate.directory / "manifest.json").exists()
    finally:
        engine.dispose()


def test_failed_new_candidate_preserves_previous_candidate(
    backup_environment: tuple[str, Minio, str, str],
    tmp_path: Path,
) -> None:
    database_url, minio, bucket, _ = backup_environment
    engine = create_engine(database_url)
    schema_path = Path(__file__).resolve().parents[2] / "database" / "schema.sql"
    object_store = MinioObjectInventory(minio, bucket)
    try:
        previous = BackupService(
            engine=engine,
            archive_writer=PostgresDumpAdapter(database_url),
            object_inspector=object_store,
            object_archiver=object_store,
            evidence_bucket=bucket,
            schema_path=schema_path,
        ).create_candidate(tmp_path)
        old_digest = hashlib.sha256((previous.directory / "database.dump").read_bytes()).hexdigest()

        class FailedWriter:
            def create_archive(self, *, snapshot_id: str, target: Path) -> str:
                target.write_bytes(b"damaged")
                raise BackupToolError("pg_dump failed")

        with pytest.raises(BackupError, match="candidate backup generation failed"):
            BackupService(
                engine=engine,
                archive_writer=FailedWriter(),
                object_inspector=object_store,
                object_archiver=object_store,
                evidence_bucket=bucket,
                schema_path=schema_path,
            ).create_candidate(tmp_path)
        assert [item for item in tmp_path.iterdir()] == [previous.directory]
        current_digest = hashlib.sha256(
            (previous.directory / "database.dump").read_bytes()
        ).hexdigest()
        assert current_digest == old_digest
    finally:
        engine.dispose()
