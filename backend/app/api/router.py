from fastapi import APIRouter

from api.routers.collection_jobs import router as collection_jobs_router
from api.routers.content_records import router as content_records_router
from api.routers.health import router as health_router
from api.routers.identity import router as identity_router
from api.routers.monitor_topics import router as monitor_topics_router
from api.routers.source_capabilities import router as source_capabilities_router

api_router = APIRouter(prefix="/api")
api_router.include_router(health_router)
api_router.include_router(identity_router)
api_router.include_router(collection_jobs_router)
api_router.include_router(content_records_router)
api_router.include_router(monitor_topics_router)
api_router.include_router(source_capabilities_router)
