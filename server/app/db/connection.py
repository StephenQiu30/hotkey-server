from __future__ import annotations

from urllib.parse import urlparse

from server.app.core.settings import settings


def _is_supported_url(database_url: str) -> bool:
    parsed = urlparse(database_url)
    return parsed.scheme.lower().startswith("postgresql")


def _shortened_scheme(database_url: str) -> str:
    parsed = urlparse(database_url)
    return parsed.scheme.split("+", 1)[0] if parsed.scheme else "unknown"


def resolve_database_url() -> str:
    """Return the configured database URL, enforcing a PostgreSQL-only policy."""

    database_url = settings.database_url
    if not _is_supported_url(database_url):
        scheme = _shortened_scheme(database_url)
        raise RuntimeError(
            f"Unsupported database URL scheme '{scheme}' in DATABASE_URL. "
            "本项目当前仅支持 PostgreSQL，请配置 DATABASE_URL 为 postgresql://（含对应 driver）."
        )
    return database_url
