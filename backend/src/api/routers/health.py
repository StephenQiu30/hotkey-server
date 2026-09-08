from fastapi import APIRouter
from fastapi.responses import JSONResponse

from api.dependencies import Health
from core.schemas import HealthView

router = APIRouter()


@router.get("/health/live", response_model=HealthView, response_model_exclude_none=True)
def live() -> HealthView:
    return HealthView(status="alive")


@router.get(
    "/health/ready",
    response_model=HealthView,
    response_model_exclude_none=True,
    responses={503: {"model": HealthView}},
)
def ready(result: Health) -> HealthView | JSONResponse:
    if result.status != "ready":
        return JSONResponse(result.model_dump(exclude_none=True), 503)
    return result
