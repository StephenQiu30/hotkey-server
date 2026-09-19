from __future__ import annotations

from collections.abc import Generator
from typing import Annotated, cast

from fastapi import Depends, Request
from sqlalchemy import text
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.orm import Session, sessionmaker

from core.errors import DependencyUnavailableError


def get_session(request: Request) -> Generator[Session, None, None]:
    factory = cast(sessionmaker[Session], request.app.state.session_factory)
    session = factory()
    try:
        yield session
    finally:
        if session.in_transaction():
            session.rollback()
        session.close()


SessionDependency = Annotated[Session, Depends(get_session)]


def require_database(session: SessionDependency) -> None:
    try:
        session.execute(text("SELECT 1"))
    except SQLAlchemyError as error:
        raise DependencyUnavailableError("database_unavailable", "数据库暂不可用") from error


DatabaseReadyDependency = Annotated[None, Depends(require_database)]
