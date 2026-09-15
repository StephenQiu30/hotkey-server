from fastapi import APIRouter

from api.dependencies import Authenticated, Sources
from sources.schemas import QueryPreview, SearchInput

router = APIRouter(prefix="/api/v1/sources", tags=["sources"])


@router.post(
    "/bluesky/query-preview", response_model=QueryPreview, operation_id="previewBlueskyQuery"
)
def query_preview(data: SearchInput, service: Sources, owner: Authenticated) -> QueryPreview:
    return service.preview(data)
