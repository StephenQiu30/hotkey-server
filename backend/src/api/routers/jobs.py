from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Header, Query

from api.dependencies import Authenticated, Jobs
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from jobs.schemas import DiagnosticInput, JobPage, JobView

router = APIRouter(prefix="/api/v1")


@router.get(
    "/jobs",
    response_model=JobPage,
    responses=error_responses(*READ_ERROR_CODES),
    tags=["jobs"],
    operation_id="listJobs",
)
def jobs(
    service: Jobs,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    cursor: UUID | None = None,
) -> JobPage:
    return service.jobs(limit, cursor)


@router.post(
    "/jobs",
    response_model=JobView,
    status_code=201,
    responses=error_responses(*WRITE_ERROR_CODES),
    tags=["jobs"],
    operation_id="createDiagnosticJob",
)
def create_job(
    data: DiagnosticInput,
    service: Jobs,
    owner: Authenticated,
    idempotency_key: Annotated[
        str, Header(min_length=1, max_length=128, pattern=r"^[a-zA-Z0-9_.:-]+$")
    ],
) -> JobView:
    return service.create_job(idempotency_key)


@router.get(
    "/jobs/{identity}",
    response_model=JobView,
    responses=error_responses(*READ_ERROR_CODES, 404),
    tags=["jobs"],
    operation_id="getJob",
)
def get_job(identity: UUID, service: Jobs, owner: Authenticated) -> JobView:
    return service.job(identity)


@router.post(
    "/jobs/{identity}/cancel",
    response_model=JobView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    tags=["jobs"],
    operation_id="cancelJob",
)
def cancel_job(identity: UUID, service: Jobs, owner: Authenticated) -> JobView:
    return service.cancel_job(identity)
