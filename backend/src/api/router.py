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
router.include_router(health.router)

api_router = APIRouter(prefix="/api")
for child in (
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
    api_router.include_router(child)

router.include_router(api_router)
