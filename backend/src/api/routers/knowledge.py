from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query

from api.dependencies import Authenticated, Knowledge
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from knowledge.schemas import KnowledgeEntryView, KnowledgePage

router = APIRouter(tags=["knowledge"])


@router.post(
    "/api/v1/analysis-runs/{identity}/knowledge-entry",
    response_model=KnowledgeEntryView,
    status_code=201,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    operation_id="publishAnalysisKnowledge",
)
def publish_analysis_knowledge(
    identity: UUID,
    service: Knowledge,
    owner: Authenticated,
) -> KnowledgeEntryView:
    return service.publish_analysis(identity)


@router.get(
    "/api/v1/knowledge",
    response_model=KnowledgePage,
    responses=error_responses(*READ_ERROR_CODES, 422),
    operation_id="searchKnowledge",
)
def search_knowledge(
    service: Knowledge,
    owner: Authenticated,
    query: Annotated[str | None, Query(min_length=2, max_length=100)] = None,
    event_id: UUID | None = None,
    limit: Annotated[int, Query(ge=1, le=20)] = 20,
) -> KnowledgePage:
    return service.search(query, event_id, limit)


@router.get(
    "/api/v1/knowledge/{identity}",
    response_model=KnowledgeEntryView,
    responses=error_responses(*READ_ERROR_CODES, 404),
    operation_id="getKnowledgeEntry",
)
def get_knowledge_entry(
    identity: UUID,
    service: Knowledge,
    owner: Authenticated,
) -> KnowledgeEntryView:
    return service.get(identity)
