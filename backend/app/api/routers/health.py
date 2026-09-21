from fastapi import APIRouter

from api.dependencies import DatabaseReadyDependency
from core.schemas import ErrorView, HealthView

router = APIRouter(tags=["系统状态"])


@router.get(
    "/health",
    operation_id="getHealth",
    summary="查询服务存活状态",
    response_model=HealthView,
    responses={500: {"model": ErrorView, "description": "服务内部异常"}},
    status_code=200,
)
def get_health() -> HealthView:
    return HealthView(status="ok")


@router.get(
    "/ready",
    operation_id="getReadiness",
    summary="查询服务就绪状态",
    response_model=HealthView,
    responses={
        500: {"model": ErrorView, "description": "服务内部异常"},
        503: {"model": ErrorView, "description": "必要依赖不可用"},
    },
    status_code=200,
)
def get_readiness(_: DatabaseReadyDependency) -> HealthView:
    return HealthView(status="ready")
