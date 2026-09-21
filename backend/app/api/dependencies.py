from __future__ import annotations

from collections.abc import Generator
from typing import Annotated, cast

from fastapi import Cookie, Depends, Header, Request, Security
from fastapi.security import APIKeyCookie
from sqlalchemy import text
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.orm import Session, sessionmaker

from core.errors import DependencyUnavailableError
from identity.services import AuthenticatedIdentity, IdentityService
from jobs.services import JobService
from monitors.services import MonitorTopicService


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
        raise DependencyUnavailableError() from error


DatabaseReadyDependency = Annotated[None, Depends(require_database)]


def get_identity_service(request: Request, session: SessionDependency) -> IdentityService:
    return IdentityService(session, request.app.state.settings)


IdentityServiceDependency = Annotated[IdentityService, Depends(get_identity_service)]


def get_job_service(session: SessionDependency) -> JobService:
    return JobService(session)


JobServiceDependency = Annotated[JobService, Depends(get_job_service)]


def get_monitor_topic_service(session: SessionDependency) -> MonitorTopicService:
    return MonitorTopicService(session)


MonitorTopicServiceDependency = Annotated[
    MonitorTopicService,
    Depends(get_monitor_topic_service),
]

_SESSION_COOKIE = APIKeyCookie(
    name="hotkey_session",
    scheme_name="SessionCookie",
    auto_error=False,
)


def require_identity_session(
    service: IdentityServiceDependency,
    session_token: Annotated[str | None, Security(_SESSION_COOKIE)],
) -> AuthenticatedIdentity:
    return service.authenticate(session_token)


AuthenticatedIdentityDependency = Annotated[
    AuthenticatedIdentity,
    Depends(require_identity_session),
]


def require_identity_csrf(
    service: IdentityServiceDependency,
    identity: AuthenticatedIdentityDependency,
    csrf_cookie: Annotated[
        str | None,
        Cookie(alias="hotkey_csrf", include_in_schema=False),
    ] = None,
    csrf_header: Annotated[str | None, Header(alias="X-HotKey-CSRF")] = None,
) -> AuthenticatedIdentity:
    service.validate_csrf(
        identity,
        csrf_cookie=csrf_cookie,
        csrf_header=csrf_header,
    )
    return identity


CsrfProtectedIdentityDependency = Annotated[
    AuthenticatedIdentity,
    Depends(require_identity_csrf),
]
