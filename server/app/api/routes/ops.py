from __future__ import annotations

from fastapi import APIRouter

from server.app.core.middleware import get_request_metrics

router = APIRouter(tags=["ops"])


@router.get("/api/ops/metrics")
def metrics_snapshot() -> dict[str, object]:
    return {"metrics": get_request_metrics()}
