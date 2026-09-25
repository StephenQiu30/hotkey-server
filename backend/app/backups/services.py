from __future__ import annotations

import hashlib
import os
import shutil
import tempfile
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Protocol
from uuid import uuid4

from sqlalchemy import Engine, func, select, text
from sqlalchemy.orm import Session

from backups.schemas import (
    BACKUP_FORMAT_VERSION,
    REQUIRED_BACKUP_SETTINGS,
    BackupDatabaseManifest,
    BackupManifest,
    BackupState,
    BackupTableCount,
    EvidenceArchiveMetadata,
    EvidenceBackupMode,
    EvidenceDeletionStatus,
    EvidenceObjectInventory,
    EvidenceObjectMetadata,
    EvidenceObjectState,
)
from db.metadata import metadata
from evidence.schemas import DeletionStatus, EvidenceBackupObjectRef
from evidence.services import EvidenceBackupInventoryService

type Clock = Callable[[], datetime]


class BackupArchiveWriter(Protocol):
    def create_archive(self, *, snapshot_id: str, target: Path) -> str: ...


class EvidenceObjectInspector(Protocol):
    def stat(self, object_name: str) -> EvidenceObjectMetadata | None: ...


class EvidenceObjectArchiver(Protocol):
    def archive(
        self,
        object_name: str,
        *,
        metadata: EvidenceObjectMetadata,
        target: Path,
    ) -> EvidenceArchiveMetadata: ...


class BackupError(RuntimeError):
    """A candidate backup could not be created safely."""


@dataclass(frozen=True, slots=True)
class BackupResult:
    directory: Path
    manifest: BackupManifest


def inventory_evidence_objects(
    references: Sequence[EvidenceBackupObjectRef],
    *,
    inspector: EvidenceObjectInspector,
    checked_at: datetime,
) -> tuple[EvidenceObjectInventory, ...]:
    inventory: list[EvidenceObjectInventory] = []
    for reference in references:
        if reference.deletion_status is DeletionStatus.COMPLETED:
            inventory.append(
                EvidenceObjectInventory(
                    resource_record_id=reference.resource_record_id,
                    object_name=reference.object_name,
                    expires_at=reference.expires_at,
                    deletion_status=EvidenceDeletionStatus(reference.deletion_status.value),
                    state=EvidenceObjectState.EXCLUDED_DELETED,
                    checked_at=checked_at,
                )
            )
            continue
        object_metadata = inspector.stat(reference.object_name)
        state = (
            EvidenceObjectState.PRESENT
            if object_metadata is not None
            else EvidenceObjectState.MISSING
        )
        inventory.append(
            EvidenceObjectInventory(
                resource_record_id=reference.resource_record_id,
                object_name=reference.object_name,
                expires_at=reference.expires_at,
                deletion_status=(
                    EvidenceDeletionStatus(reference.deletion_status.value)
                    if reference.deletion_status is not None
                    else None
                ),
                state=state,
                checked_at=checked_at,
                size_bytes=object_metadata.size_bytes if object_metadata is not None else None,
                etag=object_metadata.etag if object_metadata is not None else None,
                version_id=object_metadata.version_id if object_metadata is not None else None,
                last_modified_at=(
                    object_metadata.last_modified_at if object_metadata is not None else None
                ),
            )
        )
    return tuple(inventory)


def archive_evidence_objects(
    inventory: Sequence[EvidenceObjectInventory],
    *,
    archiver: EvidenceObjectArchiver,
    directory: Path,
) -> tuple[EvidenceObjectInventory, ...]:
    present = [item for item in inventory if item.state is EvidenceObjectState.PRESENT]
    if not present:
        return tuple(inventory)
    directory.mkdir(mode=0o700)
    directory.chmod(0o700)
    archived: list[EvidenceObjectInventory] = []
    for item in inventory:
        if item.state is not EvidenceObjectState.PRESENT:
            archived.append(item)
            continue
        if item.size_bytes is None or item.etag is None or item.last_modified_at is None:
            raise BackupError("present evidence inventory is incomplete")
        archive_key = hashlib.sha256(
            f"{item.resource_record_id}\0{item.object_name}".encode()
        ).hexdigest()
        relative_path = f"evidence/{archive_key}.blob"
        target = directory / f"{archive_key}.blob"
        result = archiver.archive(
            item.object_name,
            metadata=EvidenceObjectMetadata(
                size_bytes=item.size_bytes,
                etag=item.etag,
                version_id=item.version_id,
                last_modified_at=item.last_modified_at,
            ),
            target=target,
        )
        if result.size_bytes != item.size_bytes:
            raise BackupError("archived evidence size does not match its inventory")
        archived.append(
            item.model_copy(update={"archive_path": relative_path, "content_sha256": result.sha256})
        )
    return tuple(archived)


class BackupService:
    def __init__(
        self,
        *,
        engine: Engine,
        archive_writer: BackupArchiveWriter,
        object_inspector: EvidenceObjectInspector,
        evidence_bucket: str,
        schema_path: Path,
        object_archiver: EvidenceObjectArchiver | None = None,
        clock: Clock | None = None,
    ) -> None:
        self._engine = engine
        self._archive_writer = archive_writer
        self._object_inspector = object_inspector
        self._object_archiver = object_archiver
        self._evidence_bucket = evidence_bucket
        self._schema_path = schema_path
        self._clock = clock or (lambda: datetime.now(UTC))

    def create_candidate(self, destination: Path) -> BackupResult:
        root = self._validate_destination(destination)
        backup_id = uuid4()
        staging = Path(tempfile.mkdtemp(prefix=".hotkey-backup-", dir=root))
        staging.chmod(0o700)
        archive_path = staging / "database.dump"
        published: Path | None = None
        try:
            consistency_at, server_version, tables, references, pg_dump_version = (
                self._create_database_archive(archive_path)
            )
            self._sync_file(archive_path)
            checked_at = self._aware_now()
            evidence_objects = inventory_evidence_objects(
                references,
                inspector=self._object_inspector,
                checked_at=checked_at,
            )
            evidence_mode = EvidenceBackupMode.INVENTORY_ONLY
            if self._object_archiver is not None:
                evidence_objects = archive_evidence_objects(
                    evidence_objects,
                    archiver=self._object_archiver,
                    directory=staging / "evidence",
                )
                evidence_mode = EvidenceBackupMode.CONTENT_ARCHIVED
            manifest = BackupManifest(
                format_version=BACKUP_FORMAT_VERSION,
                backup_id=backup_id,
                state=BackupState.CANDIDATE,
                consistency_at=consistency_at,
                created_at=self._aware_now(),
                database=BackupDatabaseManifest(
                    archive_path="database.dump",
                    archive_sha256=self._sha256(archive_path),
                    archive_size_bytes=archive_path.stat().st_size,
                    schema_sha256=self._sha256(self._schema_path),
                    server_version=server_version,
                    pg_dump_version=pg_dump_version,
                    tables=tables,
                ),
                evidence_mode=evidence_mode,
                evidence_bucket=self._evidence_bucket,
                evidence_objects=evidence_objects,
                required_settings=REQUIRED_BACKUP_SETTINGS,
                secrets_included=False,
                restore_verified=False,
            )
            self._write_manifest(staging / "manifest.json", manifest)
            final = root / self._directory_name(consistency_at, backup_id.hex)
            if final.exists():
                raise BackupError("backup destination already contains the generated ID")
            os.replace(staging, final)
            published = final
            self._sync_directory(root)
            return BackupResult(directory=final, manifest=manifest)
        except Exception as error:
            shutil.rmtree(published or staging, ignore_errors=True)
            if isinstance(error, BackupError):
                raise
            raise BackupError("candidate backup generation failed") from error

    def _create_database_archive(
        self,
        archive_path: Path,
    ) -> tuple[
        datetime,
        str,
        tuple[BackupTableCount, ...],
        tuple[EvidenceBackupObjectRef, ...],
        str,
    ]:
        with (
            self._engine.connect().execution_options(
                isolation_level="REPEATABLE READ"
            ) as connection,
            connection.begin(),
        ):
            connection.execute(text("SET TRANSACTION READ ONLY"))
            snapshot = (
                connection.execute(
                    text(
                        "SELECT pg_export_snapshot() AS snapshot_id, "
                        "transaction_timestamp() AS consistency_at, "
                        "current_setting('server_version') AS server_version"
                    )
                )
                .mappings()
                .one()
            )
            tables = tuple(
                BackupTableCount(
                    name=table.name,
                    row_count=connection.execute(
                        select(func.count()).select_from(table)
                    ).scalar_one(),
                )
                for table in sorted(metadata.tables.values(), key=lambda item: item.name)
            )
            with Session(bind=connection, expire_on_commit=False) as session:
                references = EvidenceBackupInventoryService(session).list_minio_objects()
            pg_dump_version = self._archive_writer.create_archive(
                snapshot_id=snapshot["snapshot_id"],
                target=archive_path,
            )
            return (
                snapshot["consistency_at"],
                snapshot["server_version"],
                tables,
                references,
                pg_dump_version,
            )

    @staticmethod
    def _validate_destination(destination: Path) -> Path:
        try:
            root = destination.expanduser().resolve(strict=True)
        except OSError as error:
            raise BackupError("backup destination must already exist") from error
        if not root.is_dir():
            raise BackupError("backup destination must be a directory")
        if not os.access(root, os.W_OK):
            raise BackupError("backup destination is not writable")
        return root

    def _aware_now(self) -> datetime:
        now = self._clock()
        if now.utcoffset() is None:
            raise BackupError("backup clock must be timezone-aware")
        return now

    @staticmethod
    def _sha256(path: Path) -> str:
        digest = hashlib.sha256()
        with path.open("rb") as source:
            for chunk in iter(lambda: source.read(1024 * 1024), b""):
                digest.update(chunk)
        return digest.hexdigest()

    @staticmethod
    def _write_manifest(path: Path, manifest: BackupManifest) -> None:
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as target:
            target.write(manifest.model_dump_json(indent=2))
            target.write("\n")
            target.flush()
            os.fsync(target.fileno())
        BackupService._sync_directory(path.parent)

    @staticmethod
    def _sync_file(path: Path) -> None:
        descriptor = os.open(path, os.O_RDONLY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)

    @staticmethod
    def _directory_name(consistency_at: datetime, backup_id: str) -> str:
        timestamp = consistency_at.astimezone(UTC).strftime("%Y%m%dT%H%M%SZ")
        return f"hotkey-backup-{timestamp}-{backup_id}"

    @staticmethod
    def _sync_directory(path: Path) -> None:
        descriptor = os.open(path, os.O_RDONLY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
