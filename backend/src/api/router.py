from fastapi import APIRouter

from api.routers import (
    analysis,
    collection,
    contents,
    events,
    health,
    identity,
    jobs,
    knowledge,
    monitors,
    notifications,
    sources,
)

router = APIRouter()
for child in (
    health.router,
    analysis.router,
    knowledge.router,
    identity.router,
    monitors.router,
    collection.router,
    contents.router,
    events.router,
    notifications.router,
    jobs.router,
    sources.router,
):
    router.include_router(child)
