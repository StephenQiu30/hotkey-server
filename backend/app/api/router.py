from fastapi import APIRouter

from api.routers.health import router as health_router
from api.routers.identity import router as identity_router

api_router = APIRouter(prefix="/api")
api_router.include_router(health_router)
api_router.include_router(identity_router)
