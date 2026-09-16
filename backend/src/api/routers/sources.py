from fastapi import APIRouter

from api.dependencies import Authenticated, Collections, Sources
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from sources.schemas import QueryPreview, QueryPreviewInput, SourceView

router = APIRouter(prefix="/sources", tags=["sources"])


@router.get(
    "",
    response_model=list[SourceView],
    responses=error_responses(*READ_ERROR_CODES),
    operation_id="listSources",
)
def source_catalog(
    service: Sources,
    collections: Collections,
    owner: Authenticated,
) -> list[SourceView]:
    return service.catalog(collections.source_runtime_statuses())


@router.post(
    "/query-preview",
    response_model=QueryPreview,
    responses=error_responses(*WRITE_ERROR_CODES),
    operation_id="previewSourceQueries",
)
def query_preview(data: QueryPreviewInput, service: Sources, owner: Authenticated) -> QueryPreview:
    return service.preview(data)
