from typing import Annotated

from fastapi import Depends, Header, Request, Security
from fastapi.security import APIKeyCookie

from collection.services import CollectionService
from contents.services import ContentService
from core.schemas import HealthView
from db.health import readiness
from identity.schemas import Principal
from identity.services import IdentityService
from jobs.services import JobService
from monitors.services import MonitorService
from sources.services import SourceService


def identity_service(request: Request) -> IdentityService:
    return IdentityService(request.app.state.database.sessions)


def monitor_service(request: Request) -> MonitorService:
    return MonitorService(request.app.state.database.sessions)


def job_service(request: Request) -> JobService:
    return JobService(request.app.state.database.sessions)


def content_service(request: Request) -> ContentService:
    return ContentService(request.app.state.database.sessions)


def collection_service(request: Request) -> CollectionService:
    return CollectionService(
        request.app.state.database.sessions,
        evidence_configured=request.app.state.settings.s3_configured,
    )


def health_status(request: Request) -> HealthView:
    return readiness(request.app.state.database.sessions)


def source_service() -> SourceService:
    return SourceService()


Sources = Annotated[SourceService, Depends(source_service)]
Identity = Annotated[IdentityService, Depends(identity_service)]
Monitors = Annotated[MonitorService, Depends(monitor_service)]
Jobs = Annotated[JobService, Depends(job_service)]
Contents = Annotated[ContentService, Depends(content_service)]
Collections = Annotated[CollectionService, Depends(collection_service)]
Health = Annotated[HealthView, Depends(health_status)]
session_cookie = APIKeyCookie(name="hk_session", scheme_name="OwnerSession", auto_error=False)


def principal(
    request: Request,
    service: Identity,
    token: Annotated[str | None, Security(session_cookie)],
    csrf: Annotated[str | None, Header(alias="X-CSRF-Token", max_length=128)] = None,
) -> Principal:
    return service.authenticate(
        token,
        csrf,
        request.method not in {"GET", "HEAD", "OPTIONS"},
    )


Authenticated = Annotated[Principal, Depends(principal)]
