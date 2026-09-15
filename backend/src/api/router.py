from fastapi import APIRouter

from api.routers import collection, contents, health, identity, jobs, monitors, sources

router = APIRouter()
for child in (
    health.router,
    identity.router,
    monitors.router,
    collection.router,
    contents.router,
    jobs.router,
    sources.router,
):
    router.include_router(child)
