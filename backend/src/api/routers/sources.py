from fastapi import APIRouter

from api.dependencies import Authenticated, Sources
from sources.schemas import QueryPreview, QueryPreviewInput, SourceView

router = APIRouter(prefix="/api/v1/sources", tags=["sources"])


@router.get("", response_model=list[SourceView], operation_id="listSources")
def source_catalog(service: Sources, owner: Authenticated) -> list[SourceView]:
    return service.catalog()


@router.post("/query-preview", response_model=QueryPreview, operation_id="previewSourceQueries")
def query_preview(data: QueryPreviewInput, service: Sources, owner: Authenticated) -> QueryPreview:
    return service.preview(data)
