from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query

from api.dependencies import Authenticated, Monitors
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from monitors.schemas import (
    MonitorInput,
    MonitorPage,
    MonitorStateChange,
    MonitorUpdate,
    MonitorView,
)

router = APIRouter()


@router.get(
    "/monitors",
    response_model=MonitorPage,
    responses=error_responses(*READ_ERROR_CODES),
    tags=["monitoring"],
    operation_id="listMonitors",
)
def monitors(
    service: Monitors,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    cursor: UUID | None = None,
) -> MonitorPage:
    return service.monitors(limit, cursor)


@router.post(
    "/monitors",
    response_model=MonitorView,
    status_code=201,
    responses=error_responses(*WRITE_ERROR_CODES),
    tags=["monitoring"],
    operation_id="createMonitor",
)
def create_monitor(data: MonitorInput, service: Monitors, owner: Authenticated) -> MonitorView:
    return service.create_monitor(data)


@router.patch(
    "/monitors/{identity}",
    response_model=MonitorView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    tags=["monitoring"],
    operation_id="updateMonitor",
)
def update_monitor(
    identity: UUID, data: MonitorUpdate, service: Monitors, owner: Authenticated
) -> MonitorView:
    return service.update_monitor(identity, data)


@router.post(
    "/monitors/{identity}/activate",
    response_model=MonitorView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    tags=["monitoring"],
    operation_id="activateMonitor",
)
def activate_monitor(
    identity: UUID, data: MonitorStateChange, service: Monitors, owner: Authenticated
) -> MonitorView:
    return service.change_state(identity, data, "active")


@router.post(
    "/monitors/{identity}/pause",
    response_model=MonitorView,
    responses=error_responses(*WRITE_ERROR_CODES, 404, 409),
    tags=["monitoring"],
    operation_id="pauseMonitor",
)
def pause_monitor(
    identity: UUID, data: MonitorStateChange, service: Monitors, owner: Authenticated
) -> MonitorView:
    return service.change_state(identity, data, "paused")
