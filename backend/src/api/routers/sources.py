from fastapi import APIRouter

from api.dependencies import Authenticated, Sources
from sources.schemas import QueryPreview, SearchInput, SourceView

router = APIRouter(prefix="/api/v1/sources", tags=["sources"])


@router.get("", response_model=list[SourceView], operation_id="listSources")
def source_catalog(service: Sources, owner: Authenticated) -> list[SourceView]:
    return service.catalog()


@router.post(
    "/bluesky/query-preview", response_model=QueryPreview, operation_id="previewBlueskyQuery"
)
def query_preview(data: SearchInput, service: Sources, owner: Authenticated) -> QueryPreview:
    return service.preview(data)
