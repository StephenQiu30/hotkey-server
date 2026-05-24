from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import select
from sqlalchemy.orm import Session

from server.app.db.session import get_session
from server.app.core.security import require_permission
from server.app.models.check_run import CheckRun
from server.app.schemas.check_run import CheckRunCreate, CheckRunRead
from server.app.services.check_runner import run_hotspot_check

router = APIRouter(prefix="/api/check-runs", tags=["check-runs"])


@router.post("", response_model=CheckRunRead, status_code=201, dependencies=[Depends(require_permission("checkRun.manage"))])
def create_check_run(payload: CheckRunCreate, session: Session = Depends(get_session)) -> CheckRun:
    return run_hotspot_check(session, trigger_type=payload.trigger_type)


@router.get("", response_model=dict)
def list_check_runs(
    limit: int = Query(default=50, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    session: Session = Depends(get_session),
) -> dict:
    runs = list(session.scalars(select(CheckRun).order_by(CheckRun.started_at.desc()).limit(limit).offset(offset)))
    return {"items": [CheckRunRead.model_validate(run).model_dump(mode="json") for run in runs], "limit": limit, "offset": offset}


@router.get("/{run_id}", response_model=CheckRunRead)
def get_check_run(run_id: int, session: Session = Depends(get_session)) -> CheckRun:
    run = session.get(CheckRun, run_id)
    if run is None:
        raise HTTPException(status_code=404, detail="Check run not found.")
    return run
