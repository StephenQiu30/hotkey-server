from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
from collections.abc import Callable, Mapping
from pathlib import Path

from sqlalchemy.engine import make_url

type CommandRunner = Callable[..., subprocess.CompletedProcess[str]]


class BackupToolError(RuntimeError):
    """An official PostgreSQL backup tool was unavailable or failed."""


class PostgresDumpAdapter:
    def __init__(
        self,
        database_url: str,
        *,
        pg_dump_path: str | None = None,
        pg_restore_path: str | None = None,
        runner: CommandRunner = subprocess.run,
        timeout_seconds: int = 3600,
    ) -> None:
        self._database_url = make_url(database_url)
        self._pg_dump = pg_dump_path or shutil.which("pg_dump") or ""
        self._pg_restore = pg_restore_path or shutil.which("pg_restore") or ""
        self._runner = runner
        self._timeout_seconds = timeout_seconds
        if not self._pg_dump or not self._pg_restore:
            raise BackupToolError("pg_dump and pg_restore must both be installed")

    def create_archive(self, *, snapshot_id: str, target: Path) -> str:
        target.unlink(missing_ok=True)
        try:
            version = self._tool_version()
            with tempfile.TemporaryDirectory(prefix="hotkey-pgpass-") as temporary:
                passfile = Path(temporary) / "pgpass"
                self._write_passfile(passfile)
                environment = self._libpq_environment(passfile)
                self._run(
                    [
                        self._pg_dump,
                        "--format=custom",
                        "--no-owner",
                        "--no-acl",
                        f"--snapshot={snapshot_id}",
                        f"--file={target}",
                    ],
                    tool_name="pg_dump",
                    environment=environment,
                )
            if not target.is_file() or target.stat().st_size == 0:
                raise BackupToolError("pg_dump did not create a non-empty archive")
            target.chmod(0o600)
            self.check_archive(target)
            return version
        except Exception:
            target.unlink(missing_ok=True)
            raise

    def check_archive(self, archive: Path) -> None:
        self._run(
            [self._pg_restore, "--list", str(archive)],
            tool_name="pg_restore",
            environment=self._base_environment(),
        )

    def restore_archive(self, archive: Path) -> None:
        if not self._database_url.database:
            raise BackupToolError("restore target database is required")
        with tempfile.TemporaryDirectory(prefix="hotkey-pgpass-") as temporary:
            passfile = Path(temporary) / "pgpass"
            self._write_passfile(passfile)
            self._run(
                [
                    self._pg_restore,
                    f"--dbname={self._database_url.database}",
                    "--single-transaction",
                    "--no-owner",
                    "--no-acl",
                    str(archive),
                ],
                tool_name="pg_restore",
                environment=self._libpq_environment(passfile),
            )

    def _tool_version(self) -> str:
        completed = self._run(
            [self._pg_dump, "--version"],
            tool_name="pg_dump",
            environment=self._base_environment(),
        )
        version = completed.stdout.strip()
        if not version:
            raise BackupToolError("pg_dump returned an empty version")
        return version

    def _run(
        self,
        arguments: list[str],
        *,
        tool_name: str,
        environment: Mapping[str, str],
    ) -> subprocess.CompletedProcess[str]:
        try:
            completed = self._runner(
                arguments,
                check=False,
                capture_output=True,
                text=True,
                timeout=self._timeout_seconds,
                env=dict(environment),
            )
        except (OSError, subprocess.TimeoutExpired) as error:
            raise BackupToolError(f"{tool_name} could not complete") from error
        if completed.returncode != 0:
            raise BackupToolError(f"{tool_name} failed with exit code {completed.returncode}")
        return completed

    def _write_passfile(self, path: Path) -> None:
        url = self._database_url
        values = (
            url.host or "*",
            str(url.port or 5432),
            url.database or "*",
            url.username or "*",
            url.password or "",
        )
        if any("\n" in value or "\r" in value for value in values):
            raise BackupToolError("database connection fields cannot contain newlines")
        line = ":".join(self._escape_passfile(value) for value in values) + "\n"
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as handle:
            handle.write(line)
            handle.flush()
            os.fsync(handle.fileno())

    def _libpq_environment(self, passfile: Path) -> dict[str, str]:
        url = self._database_url
        environment = self._base_environment()
        environment["PGPASSFILE"] = str(passfile)
        if url.host is not None:
            environment["PGHOST"] = url.host
        if url.port is not None:
            environment["PGPORT"] = str(url.port)
        if url.database is not None:
            environment["PGDATABASE"] = url.database
        if url.username is not None:
            environment["PGUSER"] = url.username
        query_mapping = {
            "sslmode": "PGSSLMODE",
            "sslrootcert": "PGSSLROOTCERT",
            "sslcert": "PGSSLCERT",
            "sslkey": "PGSSLKEY",
            "channel_binding": "PGCHANNELBINDING",
            "connect_timeout": "PGCONNECT_TIMEOUT",
        }
        for query_key, environment_key in query_mapping.items():
            query_value = url.query.get(query_key)
            if isinstance(query_value, str):
                environment[environment_key] = query_value
        environment.setdefault("PGCONNECT_TIMEOUT", "30")
        return environment

    @staticmethod
    def _base_environment() -> dict[str, str]:
        allowed = ("PATH", "LANG", "LC_ALL", "TMPDIR")
        return {name: os.environ[name] for name in allowed if name in os.environ}

    @staticmethod
    def _escape_passfile(value: str) -> str:
        return value.replace("\\", "\\\\").replace(":", "\\:")
