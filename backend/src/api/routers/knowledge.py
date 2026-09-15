from typing import Annotated, Literal
from uuid import UUID

from fastapi import APIRouter, Query

from api.dependencies import Authenticated, Knowledge
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from knowledge.schemas import KnowledgeAnswer, KnowledgeEntryView, KnowledgePage, KnowledgeQuestion

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
    mode: Literal["exact_substring", "semantic"] = "exact_substring",
    limit: Annotated[int, Query(ge=1, le=20)] = 20,
) -> KnowledgePage:
    return service.search(query, event_id, limit, mode)


@router.post(
    "/api/v1/knowledge/query",
    response_model=KnowledgeAnswer,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 422),
    operation_id="queryKnowledge",
)
def query_knowledge(
    data: KnowledgeQuestion,
    service: Knowledge,
    owner: Authenticated,
) -> KnowledgeAnswer:
    return service.query(data)


@router.post(
    "/api/v1/knowledge/{identity}/semantic-index",
    response_model=KnowledgeEntryView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    operation_id="indexKnowledgeEntry",
)
def index_knowledge_entry(
    identity: UUID,
    service: Knowledge,
    owner: Authenticated,
) -> KnowledgeEntryView:
    return service.index(identity)


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
