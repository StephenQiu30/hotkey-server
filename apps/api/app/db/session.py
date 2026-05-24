from __future__ import annotations

from collections.abc import Generator
from functools import lru_cache

from sqlalchemy import create_engine
from sqlalchemy.orm import Session, sessionmaker

from apps.api.app.core.settings import settings
from apps.api.app.db.connection import resolve_database_url


@lru_cache(maxsize=1)
def _get_engine():
    return create_engine(resolve_database_url(), pool_pre_ping=True)


@lru_cache(maxsize=1)
def _get_session_factory():
    return sessionmaker(bind=_get_engine(), autoflush=False, autocommit=False, expire_on_commit=False)


def get_session() -> Generator[Session, None, None]:
    session = SessionLocal()
    try:
        yield session
    finally:
        session.close()


def SessionLocal() -> Session:
    return _get_session_factory()()
