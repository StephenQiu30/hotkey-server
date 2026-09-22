from __future__ import annotations

from typing import Any

from fastapi import APIRouter, Response, status

from api.dependencies import (
    AuthenticatedIdentityDependency,
    SourceConnectionServiceDependency,
)
from connections.schemas import SourcePlatformView
from core.schemas import ErrorView, PageView

router = APIRouter(prefix="/source-capabilities", tags=["来源能力"])

_RESPONSES: dict[int | str, dict[str, Any]] = {
    401: {"model": ErrorView, "description": "会话无效或已过期"},
    422: {"model": ErrorView, "description": "请求参数校验失败"},
    500: {"model": ErrorView, "description": "服务内部异常"},
}


@router.get(
    "",
    operation_id="listSourceCapabilities",
    response_model=PageView[SourcePlatformView],
    status_code=status.HTTP_200_OK,
    summary="列出来源能力",
    description="按当前 owner 返回平台目录、连接版本及手动/定时入口的持久状态。",
    responses=_RESPONSES,
)
def list_source_capabilities(
    response: Response,
    service: SourceConnectionServiceDependency,
    identity: AuthenticatedIdentityDependency,
) -> PageView[SourcePlatformView]:
    response.headers["cache-control"] = "no-store"
    return PageView(
        items=service.list_platforms(owner_id=identity.view.user.id),
        next_cursor=None,
    )
