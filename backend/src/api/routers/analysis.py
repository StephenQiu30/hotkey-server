from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query

from analysis.schemas import (
    AnalysisLabelInput,
    AnalysisRunInput,
    AnalysisRunPage,
    AnalysisRunView,
)
from api.dependencies import Analyses, Authenticated
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses

router = APIRouter(tags=["analysis"])


@router.post(
    "/api/v1/events/{identity}/analysis-runs",
    response_model=AnalysisRunView,
    status_code=201,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    operation_id="createEventAnalysisRun",
)
def create_event_analysis_run(
    identity: UUID,
    data: AnalysisRunInput,
    service: Analyses,
    owner: Authenticated,
) -> AnalysisRunView:
    return service.create(identity, data)


@router.get(
    "/api/v1/events/{identity}/analysis-runs",
    response_model=AnalysisRunPage,
    responses=error_responses(*READ_ERROR_CODES, 404),
    operation_id="listEventAnalysisRuns",
)
def list_event_analysis_runs(
    identity: UUID,
    service: Analyses,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=20)] = 5,
) -> AnalysisRunPage:
    return service.list_for_event(identity, limit)


@router.get(
    "/api/v1/analysis-runs/{identity}",
    response_model=AnalysisRunView,
    responses=error_responses(*READ_ERROR_CODES, 404),
    operation_id="getAnalysisRun",
)
def get_analysis_run(identity: UUID, service: Analyses, owner: Authenticated) -> AnalysisRunView:
    return service.get(identity)


@router.put(
    "/api/v1/analysis-runs/{identity}/samples/{sample_id}/label",
    response_model=AnalysisRunView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409, 422),
    operation_id="labelAnalysisSample",
)
def label_analysis_sample(
    identity: UUID,
    sample_id: UUID,
    data: AnalysisLabelInput,
    service: Analyses,
    owner: Authenticated,
) -> AnalysisRunView:
    return service.label(identity, sample_id, data)


@router.post(
    "/api/v1/analysis-runs/{identity}/recompute",
    response_model=AnalysisRunView,
    responses=error_responses(*WRITE_ERROR_CODES, 404),
    operation_id="recomputeAnalysisRun",
)
def recompute_analysis_run(
    identity: UUID, service: Analyses, owner: Authenticated
) -> AnalysisRunView:
    return service.recompute(identity)
