from __future__ import annotations

import stat
import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any
from uuid import uuid4

import pytest
from pydantic import ValidationError
from sqlalchemy.engine import URL

from backups.adapters.postgres import BackupToolError, PostgresDumpAdapter
from backups.schemas import (
    BackupDatabaseManifest,
    BackupManifest,
    BackupState,
    BackupTableCount,
    EvidenceBackupMode,
    EvidenceObjectMetadata,
    EvidenceObjectState,
)
from backups.services import inventory_evidence_objects
from evidence.schemas import DeletionStatus, EvidenceBackupObjectRef

NOW = datetime(2026, 9, 21, 14, tzinfo=UTC)


def _manifest(**changes: object) -> BackupManifest:
    values: dict[str, object] = {
        "format_version": "hotkey.backup.v1",
        "backup_id": uuid4(),
        "state": BackupState.CANDIDATE,
        "consistency_at": NOW,
        "created_at": NOW + timedelta(seconds=1),
        "database": BackupDatabaseManifest(
            archive_path="database.dump",
            archive_sha256="0" * 64,
            archive_size_bytes=1,
            schema_sha256="1" * 64,
            server_version="18.4",
            pg_dump_version="pg_dump (PostgreSQL) 18.4",
            tables=(BackupTableCount(name="identity_users", row_count=0),),
        ),
        "evidence_mode": EvidenceBackupMode.INVENTORY_ONLY,
        "evidence_bucket": "hotkey-evidence",
        "evidence_objects": (),
        "required_settings": (
            "HOTKEY_DATABASE_URL",
            "HOTKEY_MINIO_ENDPOINT",
            "HOTKEY_MINIO_ACCESS_KEY",
            "HOTKEY_MINIO_SECRET_KEY",
            "HOTKEY_MINIO_BUCKET",
            "HOTKEY_MINIO_SECURE",
        ),
        "secrets_included": False,
        "restore_verified": False,
    }
    values.update(changes)
    return BackupManifest(**values)


def test_candidate_manifest_cannot_claim_restore_verification() -> None:
    manifest = _manifest()

    assert manifest.state is BackupState.CANDIDATE
    assert manifest.restore_verified is False
    assert manifest.secrets_included is False
    with pytest.raises(ValidationError):
        _manifest(restore_verified=True)


def test_evidence_inventory_distinguishes_present_missing_and_deleted() -> None:
    present = EvidenceBackupObjectRef(
        resource_record_id=uuid4(),
        object_name="evidence/present.json",
        expires_at=NOW + timedelta(days=1),
        deletion_status=None,
    )
    missing = EvidenceBackupObjectRef(
        resource_record_id=uuid4(),
        object_name="evidence/missing.json",
        expires_at=NOW + timedelta(days=1),
        deletion_status=DeletionStatus.PENDING,
    )
    deleted = EvidenceBackupObjectRef(
        resource_record_id=uuid4(),
        object_name="evidence/deleted.json",
        expires_at=NOW,
        deletion_status=DeletionStatus.COMPLETED,
    )

    class Inspector:
        def __init__(self) -> None:
            self.calls: list[str] = []

        def stat(self, object_name: str) -> EvidenceObjectMetadata | None:
            self.calls.append(object_name)
            if object_name == present.object_name:
                return EvidenceObjectMetadata(
                    size_bytes=12,
                    etag="etag-1",
                    version_id=None,
                    last_modified_at=NOW,
                )
            return None

    inspector = Inspector()
    inventory = inventory_evidence_objects(
        (present, missing, deleted),
        inspector=inspector,
        checked_at=NOW,
    )

    assert [item.state for item in inventory] == [
        EvidenceObjectState.PRESENT,
        EvidenceObjectState.MISSING,
        EvidenceObjectState.EXCLUDED_DELETED,
    ]
    assert inspector.calls == [present.object_name, missing.object_name]
    assert inventory[0].size_bytes == 12
    assert inventory[1].size_bytes is None


def test_pg_dump_uses_temporary_password_file_without_secret_argv(tmp_path: Path) -> None:
    password = "p@ss:word\\end"
    database_url = URL.create(
        "postgresql+psycopg",
        username="owner",
        password=password,
        host="db.example",
        port=5432,
        database="hotkey",
    ).render_as_string(hide_password=False)
    observed: dict[str, Any] = {}

    def runner(arguments: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
        if arguments[1:] == ["--version"]:
            return subprocess.CompletedProcess(arguments, 0, "pg_dump (PostgreSQL) 18.4\n", "")
        if arguments[0] == "pg_dump":
            environment = kwargs["env"]
            assert isinstance(environment, dict)
            passfile = Path(environment["PGPASSFILE"])
            observed["passfile"] = passfile
            observed["passfile_mode"] = stat.S_IMODE(passfile.stat().st_mode)
            observed["passfile_content"] = passfile.read_text()
            observed["arguments"] = arguments
            observed["environment"] = environment
            target = Path(
                next(
                    item.removeprefix("--file=") for item in arguments if item.startswith("--file=")
                )
            )
            target.write_bytes(b"PGDMP-test")
            return subprocess.CompletedProcess(arguments, 0, "", "")
        return subprocess.CompletedProcess(arguments, 0, "archive contents", "")

    target = tmp_path / "database.dump"
    adapter = PostgresDumpAdapter(
        database_url,
        pg_dump_path="pg_dump",
        pg_restore_path="pg_restore",
        runner=runner,
    )

    version = adapter.create_archive(snapshot_id="00000003-0000001B-1", target=target)

    assert version == "pg_dump (PostgreSQL) 18.4"
    assert observed["passfile_mode"] == 0o600
    assert password not in " ".join(observed["arguments"])
    assert "PGPASSWORD" not in observed["environment"]
    assert "HOTKEY_DATABASE_URL" not in observed["environment"]
    assert "p@ss\\:word\\\\end" in observed["passfile_content"]
    assert not observed["passfile"].exists()
    assert target.read_bytes() == b"PGDMP-test"


def test_invalid_archive_is_removed_without_publishing(tmp_path: Path) -> None:
    database_url = URL.create(
        "postgresql+psycopg",
        username="owner",
        password="secret",
        host="127.0.0.1",
        database="hotkey",
    ).render_as_string(hide_password=False)

    def runner(arguments: list[str], **_: object) -> subprocess.CompletedProcess[str]:
        if arguments[1:] == ["--version"]:
            return subprocess.CompletedProcess(arguments, 0, "pg_dump (PostgreSQL) 18.4\n", "")
        if arguments[0] == "pg_dump":
            target = Path(
                next(
                    item.removeprefix("--file=") for item in arguments if item.startswith("--file=")
                )
            )
            target.write_bytes(b"broken")
            return subprocess.CompletedProcess(arguments, 0, "", "")
        return subprocess.CompletedProcess(arguments, 1, "", "invalid archive")

    target = tmp_path / "database.dump"
    adapter = PostgresDumpAdapter(
        database_url,
        pg_dump_path="pg_dump",
        pg_restore_path="pg_restore",
        runner=runner,
    )

    with pytest.raises(BackupToolError, match="pg_restore"):
        adapter.create_archive(snapshot_id="00000003-0000001B-1", target=target)

    assert not target.exists()
    assert not any(path.name.startswith("hotkey-pgpass-") for path in tmp_path.iterdir())


def test_pg_restore_uses_isolated_database_and_temporary_password_file(tmp_path: Path) -> None:
    password = "restore:secret"
    database_url = URL.create(
        "postgresql+psycopg",
        username="owner",
        password=password,
        host="db.example",
        port=5432,
        database="hotkey_restore_1234",
    ).render_as_string(hide_password=False)
    observed: dict[str, Any] = {}

    def runner(arguments: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
        environment = kwargs["env"]
        assert isinstance(environment, dict)
        passfile = Path(environment["PGPASSFILE"])
        observed["arguments"] = arguments
        observed["passfile"] = passfile
        observed["mode"] = stat.S_IMODE(passfile.stat().st_mode)
        observed["content"] = passfile.read_text()
        observed["environment"] = environment
        return subprocess.CompletedProcess(arguments, 0, "", "")

    PostgresDumpAdapter(
        database_url,
        pg_dump_path="pg_dump",
        pg_restore_path="pg_restore",
        runner=runner,
    ).restore_archive(tmp_path / "database.dump")

    assert "--dbname=hotkey_restore_1234" in observed["arguments"]
    assert "--single-transaction" in observed["arguments"]
    assert "--clean" not in observed["arguments"]
    assert "--create" not in observed["arguments"]
    assert password not in " ".join(observed["arguments"])
    assert observed["mode"] == 0o600
    assert "restore\\:secret" in observed["content"]
    assert "PGPASSWORD" not in observed["environment"]
    assert not observed["passfile"].exists()
