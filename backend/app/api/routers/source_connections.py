from __future__ import annotations

from typing import Annotated

from fastapi import APIRouter, Path, Response, status

from api.dependencies import CsrfProtectedIdentityDependency, SourceConnectionServiceDependency
from connections.schemas import SourceConnectionUpdateInput, SourceConnectionView
from core.schemas import ErrorView

router = APIRouter(prefix="/source-connections", tags=["来源能力"])


@router.put(
    "/{source_key}",
    operation_id="updateSourceConnection",
    response_model=SourceConnectionView,
    status_code=status.HTTP_200_OK,
    summary="配置或启停来源连接",
    description="仅使用服务端已配置凭据; 版本变化后必须重新验证, 历史资料保留。",
    responses={
        401: {"model": ErrorView, "description": "会话无效或已过期"},
        403: {"model": ErrorView, "description": "请求安全校验失败"},
        404: {"model": ErrorView, "description": "来源或连接不存在"},
        409: {"model": ErrorView, "description": "版本冲突或服务端尚未配置凭据"},
        422: {"model": ErrorView, "description": "请求参数校验失败"},
        500: {"model": ErrorView, "description": "服务内部异常"},
    },
)
def update_source_connection(
    source_key: Annotated[str, Path(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_-]*$")],
    payload: SourceConnectionUpdateInput,
    response: Response,
    service: SourceConnectionServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> SourceConnectionView:
    view = service.update_connection(
        owner_id=identity.view.user.id, source_key=source_key, command=payload
    )
    response.headers["cache-control"] = "no-store"
    return view
