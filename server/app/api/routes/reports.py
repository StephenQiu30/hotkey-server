from __future__ import annotations

from typing import Literal

from fastapi import APIRouter, Depends, HTTPException, Query
from fastapi.responses import HTMLResponse
from sqlalchemy import select
from sqlalchemy.orm import Session

from server.app.db.session import get_session
from server.app.core.security import require_permission
from server.app.models.report import Report
from server.app.schemas.report import ReportCreate, ReportRead
from server.app.services.reports import ReportType, generate_report, send_report, report_to_html

router = APIRouter(prefix="/api/reports", tags=["reports"])


@router.post("", response_model=ReportRead, status_code=201, dependencies=[Depends(require_permission("report.manage"))])
def create_report(payload: ReportCreate, session: Session = Depends(get_session)) -> Report:
    report = generate_report(session, report_type=payload.report_type, period_start=payload.period_start)
    if payload.send:
        send_report(session, report)
    session.commit()
    session.refresh(report)
    return report


@router.post("/{report_id}/send", response_model=ReportRead, dependencies=[Depends(require_permission("report.manage"))])
def send_existing_report(report_id: int, session: Session = Depends(get_session)) -> Report:
    report = session.get(Report, report_id)
    if report is None:
        raise HTTPException(status_code=404, detail="Report not found.")
    send_report(session, report)
    session.commit()
    session.refresh(report)
    return report


@router.get("", response_model=dict)
def list_reports(
    report_type: ReportType | None = Query(default=None),
    limit: int = Query(default=50, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    session: Session = Depends(get_session),
) -> dict:
    stmt = select(Report).order_by(Report.period_start.desc(), Report.id.desc()).limit(limit).offset(offset)
    if report_type:
        stmt = (
            select(Report)
            .where(Report.report_type == report_type)
            .order_by(Report.period_start.desc(), Report.id.desc())
            .limit(limit)
            .offset(offset)
        )
    reports = list(session.scalars(stmt))
    return {
        "items": [ReportRead.model_validate(report).model_dump(mode="json") for report in reports],
        "limit": limit,
        "offset": offset,
    }


@router.get("/{report_id}", response_model=ReportRead)
def get_report(report_id: int, session: Session = Depends(get_session)) -> Report:
    report = session.get(Report, report_id)
    if report is None:
        raise HTTPException(status_code=404, detail="Report not found.")
    return report


@router.get("/{report_id}/html", response_class=HTMLResponse)
def get_report_html(report_id: int, session: Session = Depends(get_session)) -> str:
    report = session.get(Report, report_id)
    if report is None:
        raise HTTPException(status_code=404, detail="Report not found.")
    return report_to_html(report)
