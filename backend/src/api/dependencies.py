from typing import Annotated, cast

from fastapi import Depends, Header, Request, Security
from fastapi.security import APIKeyCookie

from ai.adapters.ollama import OllamaEmbeddingProvider
from analysis.services import AnalysisService
from collection.services import CollectionService
from contents.services import ContentService
from core.schemas import HealthView
from db.health import readiness
from events.services import EventService
from identity.schemas import Principal
from identity.services import IdentityService
from jobs.services import JobService
from knowledge.services import KnowledgeService
from monitors.services import MonitorService
from notifications.services import NotificationService
from sources.services import SourceService


def identity_service(request: Request) -> IdentityService:
    return IdentityService(request.app.state.database.sessions)


def source_service(request: Request) -> SourceService:
    return cast(SourceService, request.app.state.sources)


Sources = Annotated[SourceService, Depends(source_service)]


def monitor_service(request: Request, sources: Sources) -> MonitorService:
    return MonitorService(request.app.state.database.sessions, sources)


def job_service(request: Request) -> JobService:
    factory = request.app.state.database.sessions
    settings = request.app.state.settings
    collections = CollectionService(
        factory,
        cast(SourceService, request.app.state.sources),
        evidence_configured=settings.s3_configured,
    )
    return JobService(factory, {"collect_page": collections.cancel_job})


def content_service(request: Request) -> ContentService:
    return ContentService(request.app.state.database.sessions)


def event_service(request: Request) -> EventService:
    return EventService(request.app.state.database.sessions)


def notification_service(request: Request) -> NotificationService:
    return NotificationService(request.app.state.database.sessions)


def analysis_service(request: Request) -> AnalysisService:
    return AnalysisService(request.app.state.database.sessions)


def knowledge_service(request: Request) -> KnowledgeService:
    settings = request.app.state.settings
    provider = (
        OllamaEmbeddingProvider(
            base_url=settings.embedding_base_url,
            model=settings.embedding_model,
            model_digest=settings.embedding_model_digest,
            dimensions=settings.embedding_dimensions,
            timeout_seconds=settings.embedding_timeout_seconds,
        )
        if settings.embedding_base_url is not None and settings.embedding_model_digest is not None
        else None
    )
    return KnowledgeService(request.app.state.database.sessions, provider)


def collection_service(request: Request, sources: Sources) -> CollectionService:
    return CollectionService(
        request.app.state.database.sessions,
        sources,
        evidence_configured=request.app.state.settings.s3_configured,
    )


def health_status(request: Request) -> HealthView:
    return readiness(request.app.state.database.sessions)


Identity = Annotated[IdentityService, Depends(identity_service)]
Monitors = Annotated[MonitorService, Depends(monitor_service)]
Jobs = Annotated[JobService, Depends(job_service)]
Contents = Annotated[ContentService, Depends(content_service)]
Events = Annotated[EventService, Depends(event_service)]
Notifications = Annotated[NotificationService, Depends(notification_service)]
Analyses = Annotated[AnalysisService, Depends(analysis_service)]
Knowledge = Annotated[KnowledgeService, Depends(knowledge_service)]
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
