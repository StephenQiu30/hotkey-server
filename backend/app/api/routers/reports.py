from __future__ import annotations

from datetime import date
from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, HTTPException, Query, Response, status

from api.dependencies import AuthenticatedIdentityDependency, ReportServiceDependency
from core.schemas import ErrorView, PageView
from reports.schemas import ReportDetailView, ReportKind, ReportSummaryView

router = APIRouter(prefix="/v1/reports", tags=["日报"])

_READ_RESPONSES: dict[int | str, dict[str, Any]] = {
    401: {"model": ErrorView, "description": "会话无效或已过期"},
    422: {"model": ErrorView, "description": "请求参数校验失败"},
    500: {"model": ErrorView, "description": "服务内部异常"},
}


@router.get(
    "",
    operation_id="listReports",
    response_model=PageView[ReportSummaryView],
    status_code=status.HTTP_200_OK,
    summary="列出日报",
    description=(
        "仅列出当前 owner 的定稿报告; 每个主题及时间窗只返回最新版本。"
        "日期按 Asia/Shanghai 自然日筛选。"
    ),
    responses={404: {"model": ErrorView, "description": "分页位置不存在"}, **_READ_RESPONSES},
)
def list_reports(
    response: Response,
    service: ReportServiceDependency,
    identity: AuthenticatedIdentityDependency,
    topic_id: UUID | None = None,
    date_from: date | None = None,
    date_to: date | None = None,
    kind: ReportKind = ReportKind.DAILY,
    cursor: UUID | None = None,
    limit: Annotated[int, Query(ge=1, le=50)] = 20,
) -> PageView[ReportSummaryView]:
    if date_from is not None and date_to is not None and date_from > date_to:
        raise HTTPException(status_code=422)
    items, next_cursor = service.list_reports(
        owner_id=identity.view.user.id,
        topic_id=topic_id,
        date_from=date_from,
        date_to=date_to,
        kind=kind,
        cursor=cursor,
        limit=limit,
    )
    response.headers["cache-control"] = "no-store"
    return PageView(items=items, next_cursor=next_cursor)


@router.get(
    "/{report_id}",
    operation_id="getReport",
    response_model=ReportDetailView,
    status_code=status.HTTP_200_OK,
    summary="读取日报详情",
    description="按当前 owner 读取指定定稿版本的 Markdown、窗口、截止时间和原帖引用。",
    responses={404: {"model": ErrorView, "description": "报告不存在或不可访问"}, **_READ_RESPONSES},
)
def get_report(
    report_id: UUID,
    response: Response,
    service: ReportServiceDependency,
    identity: AuthenticatedIdentityDependency,
) -> ReportDetailView:
    report = service.get_report(owner_id=identity.view.user.id, report_id=report_id)
    response.headers["cache-control"] = "no-store"
    return report
