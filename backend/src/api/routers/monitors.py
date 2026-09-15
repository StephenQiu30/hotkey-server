from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query

from api.dependencies import Authenticated, Monitors
from monitors.schemas import (
    SOURCE_NAMES,
    MonitorInput,
    MonitorPage,
    MonitorUpdate,
    MonitorView,
    SourceView,
)

router = APIRouter(prefix="/api/v1")


@router.get(
    "/sources", response_model=list[SourceView], tags=["monitoring"], operation_id="listSources"
)
def sources(owner: Authenticated) -> list[SourceView]:
    return [SourceView(id=name) for name in SOURCE_NAMES]


@router.get(
    "/monitors", response_model=MonitorPage, tags=["monitoring"], operation_id="listMonitors"
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
    tags=["monitoring"],
    operation_id="createMonitor",
)
def create_monitor(data: MonitorInput, service: Monitors, owner: Authenticated) -> MonitorView:
    return service.create_monitor(data)


@router.patch(
    "/monitors/{identity}",
    response_model=MonitorView,
    tags=["monitoring"],
    operation_id="updateMonitor",
)
def update_monitor(
    identity: UUID, data: MonitorUpdate, service: Monitors, owner: Authenticated
) -> MonitorView:
    return service.update_monitor(identity, data)
