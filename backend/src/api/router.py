from fastapi import APIRouter

from api.routers import (
    collection,
    contents,
    events,
    health,
    identity,
    jobs,
    monitors,
    notifications,
    sources,
)

router = APIRouter()
for child in (
    health.router,
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
