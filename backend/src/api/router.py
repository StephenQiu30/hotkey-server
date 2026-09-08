from fastapi import APIRouter

from api.routers import health, identity, jobs, monitors

router = APIRouter()
for child in (health.router, identity.router, monitors.router, jobs.router):
    router.include_router(child)
