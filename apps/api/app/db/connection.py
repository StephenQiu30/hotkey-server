from __future__ import annotations

import os
import sys
import tempfile
from urllib.parse import urlparse

from apps.api.app.core.settings import settings


def is_pytest_run() -> bool:
    """Return True when running under pytest, to enable local fallback policy."""

    if "PYTEST_CURRENT_TEST" in os.environ:
        return True
    if "pytest" in sys.modules:
        return True
    return any("pytest" in os.path.basename(arg).lower() for arg in sys.argv)


def _is_sqlite_url(database_url: str) -> bool:
    parsed = urlparse(database_url)
    return parsed.scheme.startswith("sqlite")


def resolve_database_url() -> str:
    """Resolve the effective DB URL for current runtime.

    In pytest, non-SQLite URLs are redirected to a local file-backed SQLite
    database to keep tests executable when PostgreSQL is unavailable in the
    local environment.
    """

    if is_pytest_run() and not _is_sqlite_url(settings.database_url):
        sqlite_file = tempfile.gettempdir() + "/hotkey_test.sqlite3"
        return f"sqlite:///{sqlite_file}"
    return settings.database_url
