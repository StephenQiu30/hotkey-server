from __future__ import annotations

import json
import os
import stat
from pathlib import Path
from typing import cast
from uuid import UUID, uuid4

from playwright.async_api import StorageState

_MAX_STATE_BYTES = 1_048_576


class BrowserStateError(Exception):
    """A browser state could not be safely stored or read."""


class BrowserStateStore:
    """Store immutable Playwright state under a controlled owner/connection/version."""

    def __init__(self, root: Path) -> None:
        self._root = root
        self._verify_directory(root)

    def save(
        self,
        *,
        owner_id: UUID,
        connection_id: UUID,
        version: int,
        state: StorageState,
    ) -> str:
        reference = self._reference(owner_id, connection_id, version)
        payload = self._encode(state)
        directory = self._directory(owner_id, connection_id, create=True)
        target = directory / f"{version}.json"
        temporary = directory / f".{version}.{uuid4().hex}.tmp"
        try:
            descriptor = os.open(
                temporary,
                os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
                0o600,
            )
            with os.fdopen(descriptor, "wb") as file:
                os.fchmod(file.fileno(), 0o600)
                file.write(payload)
                file.flush()
                os.fsync(file.fileno())
            os.link(temporary, target, follow_symlinks=False)
            self._sync_directory(directory)
        except FileExistsError as error:
            raise BrowserStateError("browser_state_already_exists") from error
        except OSError as error:
            raise BrowserStateError("browser_state_file_invalid") from error
        finally:
            temporary.unlink(missing_ok=True)
        return reference

    def load(
        self,
        *,
        owner_id: UUID,
        connection_id: UUID,
        version: int,
        reference: str,
    ) -> StorageState:
        if reference != self._reference(owner_id, connection_id, version):
            raise BrowserStateError("browser_state_reference_invalid")
        directory = self._directory(owner_id, connection_id, create=False)
        path = directory / f"{version}.json"
        try:
            descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
            with os.fdopen(descriptor, "rb") as file:
                info = os.fstat(file.fileno())
                if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600:
                    raise BrowserStateError("browser_state_file_invalid")
                payload = file.read(_MAX_STATE_BYTES + 1)
        except OSError as error:
            raise BrowserStateError("browser_state_file_invalid") from error
        return self._decode(payload)

    def _directory(self, owner_id: UUID, connection_id: UUID, *, create: bool) -> Path:
        self._verify_directory(self._root)
        directory = self._root
        for component in (owner_id.hex, connection_id.hex):
            directory = directory / component
            if create:
                try:
                    directory.mkdir(mode=0o700)
                except FileExistsError:
                    pass
                except OSError as error:
                    raise BrowserStateError("browser_state_directory_invalid") from error
            self._verify_directory(directory)
        return directory

    @staticmethod
    def _verify_directory(path: Path) -> None:
        try:
            info = path.lstat()
            canonical = path.resolve(strict=True)
        except (OSError, RuntimeError) as error:
            raise BrowserStateError("browser_state_directory_invalid") from error
        if (
            not path.is_absolute()
            or canonical != path
            or not stat.S_ISDIR(info.st_mode)
            or stat.S_IMODE(info.st_mode) != 0o700
        ):
            raise BrowserStateError("browser_state_directory_invalid")

    @staticmethod
    def _reference(owner_id: UUID, connection_id: UUID, version: int) -> str:
        if version < 1 or version > 2_147_483_647:
            raise BrowserStateError("browser_state_version_invalid")
        return f"browser-state:{owner_id.hex}/{connection_id.hex}/{version}"

    @staticmethod
    def _encode(state: StorageState) -> bytes:
        if not isinstance(state, dict):
            raise BrowserStateError("browser_state_state_invalid")
        try:
            payload = json.dumps(state, ensure_ascii=False, allow_nan=False).encode("utf-8")
        except (TypeError, ValueError) as error:
            raise BrowserStateError("browser_state_state_invalid") from error
        BrowserStateStore._decode(payload)
        return payload

    @staticmethod
    def _decode(payload: bytes) -> StorageState:
        if len(payload) > _MAX_STATE_BYTES:
            raise BrowserStateError("browser_state_state_invalid")
        try:
            state = json.loads(payload)
        except (UnicodeError, ValueError) as error:
            raise BrowserStateError("browser_state_state_invalid") from error
        if (
            not isinstance(state, dict)
            or not isinstance(state.get("cookies"), list)
            or not isinstance(state.get("origins"), list)
            or any(not isinstance(item, dict) for item in state["cookies"])
            or any(not isinstance(item, dict) for item in state["origins"])
        ):
            raise BrowserStateError("browser_state_state_invalid")
        return cast(StorageState, state)

    @staticmethod
    def _sync_directory(directory: Path) -> None:
        descriptor = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
