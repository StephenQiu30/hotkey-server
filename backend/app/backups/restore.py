from __future__ import annotations

import hashlib
import stat
import time
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Protocol
from uuid import UUID, uuid4

from pydantic import ValidationError
from sqlalchemy import create_engine, func, select, text
from sqlalchemy.engine import URL, Engine, make_url
from sqlalchemy.exc import ArgumentError, SQLAlchemyError

from backups.adapters.minio import ObjectArchiveError
from backups.adapters.postgres import BackupToolError, PostgresDumpAdapter
from backups.schemas import BackupManifest
from db.metadata import metadata


class BackupRestoreError(RuntimeError):
    """A candidate could not be safely verified in an isolated database."""


class EvidenceRestoreVerifier(Protocol):
    def verify(self, *, manifest: BackupManifest, candidate: Path) -> int: ...


@dataclass(frozen=True, slots=True)
class BackupRestoreResult:
    backup_id: UUID
    table_count: int
    duration_seconds: float
    database_restored: bool
    evidence_objects_verified: int


class BackupRestoreService:
    def __init__(
        self,
        *,
        source_database_url: str | URL,
        isolation_database_url: str | URL,
        schema_path: Path,
        evidence_restore_verifier: EvidenceRestoreVerifier | None = None,
    ) -> None:
        try:
            self._source_url = make_url(source_database_url)
            self._isolation_url = make_url(isolation_database_url)
        except ArgumentError as error:
            raise BackupRestoreError("database connection settings are invalid") from error
        if any(
            url.drivername != "postgresql+psycopg" or not url.database
            for url in (self._source_url, self._isolation_url)
        ):
            raise BackupRestoreError("PostgreSQL source and isolation databases are required")
        self._schema_path = schema_path
        self._evidence_restore_verifier = evidence_restore_verifier

    def verify(self, candidate: Path) -> BackupRestoreResult:
        started = time.monotonic()
        manifest, archive = self._check_candidate(candidate)
        if (self._source_url.host, self._source_url.port) != (
            self._isolation_url.host,
            self._isolation_url.port,
        ) or self._source_url.database == self._isolation_url.database:
            raise BackupRestoreError(
                "isolation connection must use another database on the source server"
            )
        evidence_count = sum(item.archive_path is not None for item in manifest.evidence_objects)
        if evidence_count and self._evidence_restore_verifier is None:
            raise BackupRestoreError("MinIO evidence restore verification is required")

        database_name = f"hotkey_restore_{uuid4().hex}"
        admin_engine = create_engine(self._isolation_url)
        created = False
        try:
            with admin_engine.connect().execution_options(
                isolation_level="AUTOCOMMIT"
            ) as connection:
                connection.exec_driver_sql(f'CREATE DATABASE "{database_name}"')
            created = True
            target_url = self._isolation_url.set(database=database_name)
            PostgresDumpAdapter(target_url.render_as_string(hide_password=False)).restore_archive(
                archive
            )
            target_engine = create_engine(target_url)
            try:
                self._check_restored_data(target_engine, manifest)
            finally:
                target_engine.dispose()
            evidence_objects_verified = 0
            if evidence_count and self._evidence_restore_verifier is not None:
                try:
                    evidence_objects_verified = self._evidence_restore_verifier.verify(
                        manifest=manifest,
                        candidate=candidate,
                    )
                except ObjectArchiveError as error:
                    raise BackupRestoreError(str(error)) from error
            return BackupRestoreResult(
                backup_id=manifest.backup_id,
                table_count=len(manifest.database.tables),
                duration_seconds=time.monotonic() - started,
                database_restored=True,
                evidence_objects_verified=evidence_objects_verified,
            )
        except (BackupToolError, SQLAlchemyError, OSError) as error:
            raise BackupRestoreError(
                "isolated database restore or reconciliation failed"
            ) from error
        finally:
            try:
                if created:
                    with admin_engine.connect().execution_options(
                        isolation_level="AUTOCOMMIT"
                    ) as connection:
                        connection.exec_driver_sql(f'DROP DATABASE "{database_name}"')
            except SQLAlchemyError as error:
                raise BackupRestoreError(
                    f"temporary restore database cleanup failed: {database_name}"
                ) from error
            finally:
                admin_engine.dispose()

    def _check_candidate(self, candidate: Path) -> tuple[BackupManifest, Path]:
        try:
            if not candidate.is_dir() or candidate.is_symlink():
                raise BackupRestoreError("candidate directory is invalid")
            if stat.S_IMODE(candidate.stat().st_mode) != 0o700:
                raise BackupRestoreError("candidate directory permissions are invalid")
            manifest_path = candidate / "manifest.json"
            archive = candidate / "database.dump"
            for path in (manifest_path, archive):
                mode = path.lstat().st_mode
                if not stat.S_ISREG(mode) or stat.S_IMODE(mode) != 0o600:
                    raise BackupRestoreError("candidate file permissions or type are invalid")
            manifest = BackupManifest.model_validate_json(manifest_path.read_bytes())
            timestamp = manifest.consistency_at.astimezone(UTC).strftime("%Y%m%dT%H%M%SZ")
            expected_name = f"hotkey-backup-{timestamp}-{manifest.backup_id.hex}"
            if candidate.name != expected_name:
                raise BackupRestoreError("candidate directory identity does not match manifest")
            if self._sha256(self._schema_path) != manifest.database.schema_sha256:
                raise BackupRestoreError("candidate schema does not match current schema")
            if archive.stat().st_size != manifest.database.archive_size_bytes:
                raise BackupRestoreError("candidate archive size does not match manifest")
            if self._sha256(archive) != manifest.database.archive_sha256:
                raise BackupRestoreError("candidate archive digest does not match manifest")
            if [item.name for item in manifest.database.tables] != sorted(metadata.tables):
                raise BackupRestoreError("candidate table inventory is incomplete")
            expected_archive_paths = {
                item.archive_path
                for item in manifest.evidence_objects
                if item.archive_path is not None
            }
            expected_root_entries = {"manifest.json", "database.dump"}
            if expected_archive_paths:
                evidence_directory = candidate / "evidence"
                directory_status = evidence_directory.lstat()
                if (
                    not stat.S_ISDIR(directory_status.st_mode)
                    or stat.S_IMODE(directory_status.st_mode) != 0o700
                ):
                    raise BackupRestoreError("candidate evidence directory is invalid")
                expected_names = {Path(item).name for item in expected_archive_paths}
                if {path.name for path in evidence_directory.iterdir()} != expected_names:
                    raise BackupRestoreError("candidate evidence inventory is incomplete")
                for item in manifest.evidence_objects:
                    if item.archive_path is None:
                        continue
                    evidence_file = candidate / item.archive_path
                    evidence_status = evidence_file.lstat()
                    if (
                        not stat.S_ISREG(evidence_status.st_mode)
                        or stat.S_IMODE(evidence_status.st_mode) != 0o600
                        or evidence_status.st_size != item.size_bytes
                        or self._sha256(evidence_file) != item.content_sha256
                    ):
                        raise BackupRestoreError("candidate evidence content is invalid")
                expected_root_entries.add("evidence")
            if {path.name for path in candidate.iterdir()} != expected_root_entries:
                raise BackupRestoreError("candidate contains unexpected files")
            PostgresDumpAdapter(
                self._isolation_url.render_as_string(hide_password=False)
            ).check_archive(archive)
            return manifest, archive
        except (OSError, ValidationError, BackupToolError) as error:
            raise BackupRestoreError("candidate manifest or archive validation failed") from error

    @staticmethod
    def _check_restored_data(engine: Engine, manifest: BackupManifest) -> None:
        try:
            with engine.connect() as connection:
                transaction = connection.begin()
                try:
                    for item in manifest.database.tables:
                        actual = connection.execute(
                            select(func.count()).select_from(metadata.tables[item.name])
                        ).scalar_one()
                        if actual != item.row_count:
                            raise BackupRestoreError("restored table counts do not match candidate")

                    owner_id = connection.execute(
                        text("SELECT id FROM identity_users LIMIT 1")
                    ).scalar_one_or_none()
                    if owner_id is None:
                        owner_id = uuid4()
                        connection.execute(
                            text(
                                "INSERT INTO identity_users "
                                "(id, username, password_hash, credential_version, "
                                "created_at, updated_at) "
                                "VALUES (:id, 'restore-probe', 'restore-probe', 1, :now, :now)"
                            ),
                            {"id": owner_id, "now": datetime.now(UTC)},
                        )
                    else:
                        connection.execute(
                            text(
                                "UPDATE identity_users SET password_hash = 'restore-probe' "
                                "WHERE id = :id"
                            ),
                            {"id": owner_id},
                        )
                    if (
                        connection.execute(
                            text("SELECT password_hash FROM identity_users WHERE id = :id"),
                            {"id": owner_id},
                        ).scalar_one()
                        != "restore-probe"
                    ):
                        raise BackupRestoreError("isolated write probe could not be read")
                finally:
                    transaction.rollback()
        except SQLAlchemyError as error:
            raise BackupRestoreError("restored data read or write probe failed") from error

    @staticmethod
    def _sha256(path: Path) -> str:
        digest = hashlib.sha256()
        with path.open("rb") as source:
            for chunk in iter(lambda: source.read(1024 * 1024), b""):
                digest.update(chunk)
        return digest.hexdigest()
