from __future__ import annotations

from typing import Any
from uuid import UUID

from fastapi import APIRouter, Response, status

from api.dependencies import (
    AuthenticatedIdentityDependency,
    CsrfProtectedIdentityDependency,
    MonitorTopicServiceDependency,
)
from core.schemas import ErrorView
from monitors.schemas import MonitorTopicCreateInput, MonitorTopicUpdateInput, MonitorTopicView

router = APIRouter(prefix="/topics", tags=["监控主题"])

_READ_RESPONSES: dict[int | str, dict[str, Any]] = {
    401: {"model": ErrorView, "description": "会话无效或已过期"},
    404: {"model": ErrorView, "description": "主题不存在或不可访问"},
    422: {"model": ErrorView, "description": "请求参数校验失败"},
    500: {"model": ErrorView, "description": "服务内部异常"},
}

_WRITE_RESPONSES: dict[int | str, dict[str, Any]] = {
    401: {"model": ErrorView, "description": "会话无效或已过期"},
    403: {"model": ErrorView, "description": "请求安全校验失败"},
    404: {"model": ErrorView, "description": "主题不存在或不可访问"},
    409: {"model": ErrorView, "description": "主题版本已变更"},
    422: {"model": ErrorView, "description": "请求参数校验失败"},
    500: {"model": ErrorView, "description": "服务内部异常"},
}


@router.post(
    "",
    operation_id="createMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_201_CREATED,
    summary="创建监控主题",
    description="保存本地匹配规则版本; 来源未选定时主题保持暂停。",
    responses=_WRITE_RESPONSES,
)
def create_monitor_topic(
    payload: MonitorTopicCreateInput,
    response: Response,
    service: MonitorTopicServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> MonitorTopicView:
    topic = service.create_topic(owner_id=identity.view.user.id, command=payload)
    response.headers["location"] = f"/api/topics/{topic.id}"
    response.headers["cache-control"] = "no-store"
    return topic


@router.get(
    "/{topic_id}",
    operation_id="getMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_200_OK,
    summary="读取监控主题",
    description="按当前会话 owner 读取主题和当前不可变规则版本。",
    responses=_READ_RESPONSES,
)
def get_monitor_topic(
    topic_id: UUID,
    response: Response,
    service: MonitorTopicServiceDependency,
    identity: AuthenticatedIdentityDependency,
) -> MonitorTopicView:
    topic = service.get_topic(owner_id=identity.view.user.id, topic_id=topic_id)
    response.headers["cache-control"] = "no-store"
    return topic


@router.patch(
    "/{topic_id}",
    operation_id="updateMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_200_OK,
    summary="编辑监控主题",
    description="使用 expected_version 防止覆盖并发修改; 规则变化创建新版本。",
    responses=_WRITE_RESPONSES,
)
def update_monitor_topic(
    topic_id: UUID,
    payload: MonitorTopicUpdateInput,
    response: Response,
    service: MonitorTopicServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> MonitorTopicView:
    topic = service.update_topic(
        owner_id=identity.view.user.id,
        topic_id=topic_id,
        command=payload,
    )
    response.headers["cache-control"] = "no-store"
    return topic
