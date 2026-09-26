from __future__ import annotations

from typing import Annotated, Any

from fastapi import APIRouter, Path, Query, Response, status

from api.dependencies import AuthenticatedIdentityDependency, HotlistServiceDependency
from content.schemas import HotlistSnapshotView, HotlistSourceView
from core.schemas import ErrorView, PageView

router = APIRouter(prefix="/hotlists", tags=["热榜"])

_READ_RESPONSES: dict[int | str, dict[str, Any]] = {
    401: {"model": ErrorView, "description": "会话无效或已过期"},
    422: {"model": ErrorView, "description": "请求参数校验失败"},
    500: {"model": ErrorView, "description": "服务内部异常"},
}


@router.get(
    "/sources",
    operation_id="listHotlistSources",
    response_model=PageView[HotlistSourceView],
    status_code=status.HTTP_200_OK,
    summary="列出已应用热榜来源",
    description="仅列出当前用户已应用的热榜来源及最近快照时间。读取不会访问 RSSHub。",
    responses=_READ_RESPONSES,
)
def list_hotlist_sources(
    response: Response,
    service: HotlistServiceDependency,
    identity: AuthenticatedIdentityDependency,
) -> PageView[HotlistSourceView]:
    items = service.list_sources(owner_id=identity.view.user.id)
    response.headers["cache-control"] = "no-store"
    return PageView(items=items, next_cursor=None)


@router.get(
    "/{source_key}",
    operation_id="getHotlistSnapshot",
    response_model=HotlistSnapshotView,
    status_code=status.HTTP_200_OK,
    summary="读取最新热榜快照",
    description="读取当前用户最近一次快照及与上次快照的排名变化。按排名游标分页。",
    responses={
        404: {"model": ErrorView, "description": "来源未应用或尚无快照"},
        **_READ_RESPONSES,
    },
)
def get_hotlist_snapshot(
    source_key: Annotated[
        str, Path(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_-]{0,63}$")
    ],
    response: Response,
    service: HotlistServiceDependency,
    identity: AuthenticatedIdentityDependency,
    cursor: Annotated[int | None, Query(ge=1, le=100)] = None,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
) -> HotlistSnapshotView:
    snapshot = service.get_latest(
        owner_id=identity.view.user.id,
        source_key=source_key,
        cursor=cursor,
        limit=limit,
    )
    response.headers["cache-control"] = "no-store"
    return snapshot
