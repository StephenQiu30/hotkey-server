from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query

from api.dependencies import Authenticated, Collections
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from collection.schemas import (
    CollectionRunBatchInput,
    CollectionRunBatchView,
    CollectionRunPage,
    CollectionRunView,
    MonitorRunRequest,
)

router = APIRouter(tags=["collection"])


@router.post(
    "/monitors/{identity}/runs",
    response_model=CollectionRunBatchView,
    status_code=201,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409, 429),
    operation_id="createCollectionRun",
)
def create_collection_run(
    identity: UUID,
    data: MonitorRunRequest,
    service: Collections,
    owner: Authenticated,
) -> CollectionRunBatchView:
    return service.create_monitor_runs(
        CollectionRunBatchInput(
            monitor_id=identity,
            **data.model_dump(),
        )
    )


@router.get(
    "/collection-runs",
    response_model=CollectionRunPage,
    responses=error_responses(*READ_ERROR_CODES),
    operation_id="listCollectionRuns",
)
def list_collection_runs(
    service: Collections,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    cursor: Annotated[str | None, Query(max_length=100)] = None,
) -> CollectionRunPage:
    return service.runs(limit, cursor)


@router.get(
    "/collection-runs/{identity}",
    response_model=CollectionRunView,
    responses=error_responses(*READ_ERROR_CODES, 404),
    operation_id="getCollectionRun",
)
def get_collection_run(
    identity: UUID, service: Collections, owner: Authenticated
) -> CollectionRunView:
    return service.run(identity)
