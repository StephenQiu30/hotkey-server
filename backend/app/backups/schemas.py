from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

BACKUP_FORMAT_VERSION: Literal["hotkey.backup.v1"] = "hotkey.backup.v1"
REQUIRED_BACKUP_SETTINGS = (
    "HOTKEY_DATABASE_URL",
    "HOTKEY_MINIO_ENDPOINT",
    "HOTKEY_MINIO_ACCESS_KEY",
    "HOTKEY_MINIO_SECRET_KEY",
    "HOTKEY_MINIO_BUCKET",
    "HOTKEY_MINIO_SECURE",
)


class BackupState(StrEnum):
    CANDIDATE = "candidate"


class EvidenceBackupMode(StrEnum):
    INVENTORY_ONLY = "inventory_only"


class EvidenceObjectState(StrEnum):
    PRESENT = "present"
    MISSING = "missing"
    EXCLUDED_DELETED = "excluded_deleted"


class EvidenceDeletionStatus(StrEnum):
    PENDING = "pending"
    COMPLETED = "completed"
    FAILED = "failed"


class BackupTableCount(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    name: str = Field(min_length=1, max_length=63, pattern=r"^[a-z][a-z0-9_]{0,62}$")
    row_count: int = Field(ge=0)


class EvidenceObjectMetadata(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    size_bytes: int = Field(ge=0)
    etag: str = Field(min_length=1, max_length=256)
    version_id: str | None = Field(default=None, min_length=1, max_length=256)
    last_modified_at: datetime

    @field_validator("last_modified_at")
    @classmethod
    def validate_last_modified_at(cls, value: datetime) -> datetime:
        if value.utcoffset() is None:
            raise ValueError("last_modified_at must be timezone-aware")
        return value


class EvidenceObjectInventory(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    resource_record_id: UUID
    object_name: str = Field(min_length=1, max_length=1024)
    expires_at: datetime
    deletion_status: EvidenceDeletionStatus | None = None
    state: EvidenceObjectState
    checked_at: datetime
    size_bytes: int | None = Field(default=None, ge=0)
    etag: str | None = Field(default=None, min_length=1, max_length=256)
    version_id: str | None = Field(default=None, min_length=1, max_length=256)
    last_modified_at: datetime | None = None

    @model_validator(mode="after")
    def validate_state(self) -> EvidenceObjectInventory:
        timestamps = (self.expires_at, self.checked_at, self.last_modified_at)
        if any(value is not None and value.utcoffset() is None for value in timestamps):
            raise ValueError("evidence inventory timestamps must be timezone-aware")
        metadata = (self.size_bytes, self.etag, self.last_modified_at)
        if self.state is EvidenceObjectState.PRESENT and any(item is None for item in metadata):
            raise ValueError("present evidence objects require size, etag, and last_modified_at")
        if self.state is not EvidenceObjectState.PRESENT and any(
            item is not None for item in (*metadata, self.version_id)
        ):
            raise ValueError("non-present evidence objects cannot include object metadata")
        if (
            self.state is EvidenceObjectState.EXCLUDED_DELETED
            and self.deletion_status is not EvidenceDeletionStatus.COMPLETED
        ):
            raise ValueError("excluded evidence objects require completed deletion")
        return self


class BackupDatabaseManifest(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    archive_path: Literal["database.dump"]
    archive_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    archive_size_bytes: int = Field(gt=0)
    schema_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    server_version: str = Field(min_length=1, max_length=128)
    pg_dump_version: str = Field(min_length=1, max_length=128)
    tables: tuple[BackupTableCount, ...] = Field(min_length=1)

    @field_validator("tables")
    @classmethod
    def validate_tables(
        cls,
        value: tuple[BackupTableCount, ...],
    ) -> tuple[BackupTableCount, ...]:
        names = [table.name for table in value]
        if names != sorted(names) or len(names) != len(set(names)):
            raise ValueError("backup tables must be unique and sorted")
        return value


class BackupManifest(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    format_version: Literal["hotkey.backup.v1"]
    backup_id: UUID
    state: BackupState
    consistency_at: datetime
    created_at: datetime
    database: BackupDatabaseManifest
    evidence_mode: EvidenceBackupMode
    evidence_bucket: str = Field(min_length=3, max_length=63)
    evidence_objects: tuple[EvidenceObjectInventory, ...]
    required_settings: tuple[str, ...]
    secrets_included: Literal[False]
    restore_verified: Literal[False]

    @model_validator(mode="after")
    def validate_candidate(self) -> BackupManifest:
        if self.consistency_at.utcoffset() is None or self.created_at.utcoffset() is None:
            raise ValueError("backup timestamps must be timezone-aware")
        if self.created_at < self.consistency_at:
            raise ValueError("created_at cannot precede consistency_at")
        if self.required_settings != REQUIRED_BACKUP_SETTINGS:
            raise ValueError("required_settings must contain only stable setting names")
        if any(
            item.checked_at < self.consistency_at or item.checked_at > self.created_at
            for item in self.evidence_objects
        ):
            raise ValueError("evidence checks must follow the snapshot and precede creation")
        object_keys = [
            (item.resource_record_id, item.object_name) for item in self.evidence_objects
        ]
        if object_keys != sorted(object_keys, key=lambda item: (str(item[0]), item[1])):
            raise ValueError("evidence objects must be sorted")
        if len(object_keys) != len(set(object_keys)):
            raise ValueError("evidence objects must be unique")
        return self
