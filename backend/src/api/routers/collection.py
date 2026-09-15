from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Header, Query

from api.dependencies import Authenticated, Collections
from collection.schemas import (
    CollectionRunInput,
    CollectionRunPage,
    CollectionRunRequest,
    CollectionRunView,
)

router = APIRouter(prefix="/api/v1", tags=["collection"])


@router.post(
    "/monitors/{identity}/runs",
    response_model=CollectionRunView,
    status_code=201,
    operation_id="createCollectionRun",
)
def create_collection_run(
    identity: UUID,
    data: CollectionRunRequest,
    service: Collections,
    owner: Authenticated,
    idempotency_key: Annotated[
        str, Header(min_length=1, max_length=128, pattern=r"^[a-zA-Z0-9_.:-]+$")
    ],
) -> CollectionRunView:
    return service.create_run(
        CollectionRunInput(
            monitor_id=identity,
            idempotency_key=idempotency_key,
            **data.model_dump(),
        )
    )


@router.get(
    "/collection-runs",
    response_model=CollectionRunPage,
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
    operation_id="getCollectionRun",
)
def get_collection_run(
    identity: UUID, service: Collections, owner: Authenticated
) -> CollectionRunView:
    return service.run(identity)
