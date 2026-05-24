from __future__ import annotations

import time
from pathlib import Path

from sqlalchemy import create_engine
from sqlalchemy.exc import OperationalError

from apps.api.app.core.settings import settings
from apps.api.app.db.base import Base
from apps.api.app.db.connection import resolve_database_url

PROJECT_ROOT = Path(__file__).resolve().parents[4]
SCHEMA_PATH = PROJECT_ROOT / "sql" / "001_init_schema.sql"


def initialize_database() -> None:
    """Initialize database from schema.

    For tests, automatically fall back to SQLAlchemy metadata creation with
    SQLite to avoid external PostgreSQL dependency while preserving existing
    PostgreSQL schema behavior in production.
    """
    database_url = resolve_database_url()
    if database_url.startswith("sqlite:///"):
        create_schema_from_models_for_dev(database_url)
        return

    schema_statements = [
        statement.strip()
        for statement in SCHEMA_PATH.read_text(encoding="utf-8").split(";")
        if statement.strip()
    ]
    engine = create_engine(database_url, pool_pre_ping=True)

    for attempt in range(1, settings.database_init_retries + 1):
        try:
            with engine.begin() as connection:
                for statement in schema_statements:
                    connection.exec_driver_sql(statement)
            return
        except OperationalError:
            if attempt >= settings.database_init_retries:
                raise
            time.sleep(settings.database_init_retry_seconds)


def create_schema_from_models_for_dev(database_url: str | None = None) -> None:
    """Development fallback only; sql/001_init_schema.sql remains authoritative."""
    import apps.api.app.models  # noqa: F401

    database_url = database_url or resolve_database_url()
    engine = create_engine(database_url, pool_pre_ping=True)
    Base.metadata.create_all(bind=engine)


if __name__ == "__main__":
    initialize_database()
