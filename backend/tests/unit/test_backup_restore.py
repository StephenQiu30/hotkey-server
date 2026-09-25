from __future__ import annotations

import hashlib
import stat
import subprocess
from datetime import UTC, datetime, timedelta
from pathlib import Path
from types import SimpleNamespace
from typing import Any
from uuid import uuid4

import pytest
from pydantic import ValidationError
from sqlalchemy.engine import URL

from backups.adapters.minio import MinioEvidenceRestoreVerifier, MinioObjectInventory
from backups.adapters.postgres import BackupToolError, PostgresDumpAdapter
from backups.schemas import (
    BackupDatabaseManifest,
    BackupManifest,
    BackupState,
    BackupTableCount,
    EvidenceArchiveMetadata,
    EvidenceBackupMode,
    EvidenceObjectInventory,
    EvidenceObjectMetadata,
    EvidenceObjectState,
)
from backups.services import archive_evidence_objects, inventory_evidence_objects
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


def test_minio_archive_pins_source_version_and_records_content_digest(tmp_path: Path) -> None:
    content = b"stable evidence bytes"

    class Response:
        def __init__(self) -> None:
            self.remaining = content
            self.closed = False
            self.released = False

        def read(self, size: int) -> bytes:
            chunk, self.remaining = self.remaining[:size], self.remaining[size:]
            return chunk

        def close(self) -> None:
            self.closed = True

        def release_conn(self) -> None:
            self.released = True

    class Client:
        def __init__(self) -> None:
            self.response = Response()
            self.request: dict[str, object] = {}

        def get_object(
            self,
            bucket_name: str,
            object_name: str,
            **kwargs: object,
        ) -> Response:
            self.request = {"bucket_name": bucket_name, "object_name": object_name, **kwargs}
            return self.response

    client = Client()
    target = tmp_path / "evidence" / "object.blob"
    target.parent.mkdir(mode=0o700)
    metadata = EvidenceObjectMetadata(
        size_bytes=len(content),
        etag="source-etag",
        version_id="source-version",
        last_modified_at=NOW,
    )

    result = MinioObjectInventory(client, "hotkey-evidence").archive(
        "evidence/private-key.json",
        metadata=metadata,
        target=target,
    )

    assert target.read_bytes() == content
    assert stat.S_IMODE(target.stat().st_mode) == 0o600
    assert result.size_bytes == len(content)
    assert result.sha256 == hashlib.sha256(content).hexdigest()
    assert client.request["version_id"] == "source-version"
    assert client.request["request_headers"] == {"If-Match": '"source-etag"'}
    assert client.response.closed is True
    assert client.response.released is True


def test_minio_restore_verifier_reads_back_and_removes_only_created_version(
    tmp_path: Path,
) -> None:
    content = b"restored evidence bytes"
    archive_path = "evidence/" + "a" * 64 + ".blob"
    archive_file = tmp_path / archive_path
    archive_file.parent.mkdir(mode=0o700)
    archive_file.write_bytes(content)
    archive_file.chmod(0o600)

    class Response:
        def __init__(self, data: bytes) -> None:
            self.remaining = data

        def read(self, size: int) -> bytes:
            chunk, self.remaining = self.remaining[:size], self.remaining[size:]
            return chunk

        def close(self) -> None:
            pass

        def release_conn(self) -> None:
            pass

    class Client:
        def __init__(self) -> None:
            self.objects: dict[str, bytes] = {}
            self.removed: list[tuple[str, str | None]] = []
            self.statted_version: str | None = None

        def fput_object(self, bucket_name: str, object_name: str, file_path: str) -> object:
            self.objects[object_name] = Path(file_path).read_bytes()
            return SimpleNamespace(version_id="restore-version")

        def stat_object(
            self,
            bucket_name: str,
            object_name: str,
            **kwargs: object,
        ) -> object:
            self.statted_version = kwargs.get("version_id")
            return SimpleNamespace(size=len(self.objects[object_name]))

        def get_object(self, bucket_name: str, object_name: str, **kwargs: object) -> Response:
            return Response(self.objects[object_name])

        def remove_object(
            self,
            bucket_name: str,
            object_name: str,
            version_id: str | None = None,
        ) -> None:
            self.removed.append((object_name, version_id))
            self.objects.pop(object_name, None)

    client = Client()
    item = {
        "resource_record_id": uuid4(),
        "object_name": "private/evidence-name.json",
        "expires_at": NOW + timedelta(days=1),
        "deletion_status": None,
        "state": EvidenceObjectState.PRESENT,
        "checked_at": NOW,
        "size_bytes": len(content),
        "etag": "source-etag",
        "version_id": "source-version",
        "last_modified_at": NOW,
        "archive_path": archive_path,
        "content_sha256": hashlib.sha256(content).hexdigest(),
    }
    manifest = _manifest(
        evidence_mode=EvidenceBackupMode.CONTENT_ARCHIVED,
        evidence_objects=(item,),
    )

    verified = MinioEvidenceRestoreVerifier(client, "hotkey-evidence").verify(
        manifest=manifest,
        candidate=tmp_path,
    )

    assert verified == 1
    assert len(client.removed) == 1
    restored_name, restored_version = client.removed[0]
    assert restored_name.startswith("hotkey-restore-verification/")
    assert "private/evidence-name.json" not in restored_name
    assert restored_version == "restore-version"
    assert client.statted_version == "restore-version"
    assert restored_name not in client.objects


def test_content_archive_records_safe_path_and_sha256(tmp_path: Path) -> None:
    content = b"content archived from a stable object version"
    item = EvidenceObjectInventory(
        resource_record_id=uuid4(),
        object_name="private/untrusted/../../evidence.json",
        expires_at=NOW + timedelta(days=1),
        state=EvidenceObjectState.PRESENT,
        checked_at=NOW,
        size_bytes=len(content),
        etag="etag-2",
        version_id="version-2",
        last_modified_at=NOW,
    )

    class Archiver:
        target: Path | None = None

        def archive(
            self,
            object_name: str,
            *,
            metadata: EvidenceObjectMetadata,
            target: Path,
        ) -> EvidenceArchiveMetadata:
            self.target = target
            target.write_bytes(content)
            return EvidenceArchiveMetadata(
                size_bytes=len(content),
                sha256=hashlib.sha256(content).hexdigest(),
            )

    archiver = Archiver()
    archived = archive_evidence_objects(
        (item,),
        archiver=archiver,
        directory=tmp_path / "evidence",
    )

    assert archived[0].archive_path is not None
    assert archived[0].archive_path.startswith("evidence/")
    assert "untrusted" not in archived[0].archive_path
    assert archived[0].content_sha256 == hashlib.sha256(content).hexdigest()
    assert archiver.target == tmp_path / archived[0].archive_path
    assert archiver.target is not None
    assert archiver.target.read_bytes() == content
    assert stat.S_IMODE((tmp_path / "evidence").stat().st_mode) == 0o700


def test_content_archive_manifest_requires_content_for_present_objects() -> None:
    item = EvidenceObjectInventory(
        resource_record_id=uuid4(),
        object_name="evidence/present.json",
        expires_at=NOW + timedelta(days=1),
        state=EvidenceObjectState.PRESENT,
        checked_at=NOW,
        size_bytes=1,
        etag="etag-3",
        last_modified_at=NOW,
    )

    with pytest.raises(ValidationError, match="require archived content"):
        _manifest(
            evidence_mode=EvidenceBackupMode.CONTENT_ARCHIVED,
            evidence_objects=(item,),
        )


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
