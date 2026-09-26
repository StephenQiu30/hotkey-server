from __future__ import annotations

from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, Query, Response, status

from api.dependencies import (
    AuthenticatedIdentityDependency,
    CsrfProtectedIdentityDependency,
    MonitorTopicServiceDependency,
)
from core.schemas import ErrorView, PageView
from monitors.schemas import (
    MonitorTopicCreateInput,
    MonitorTopicPreviewInput,
    MonitorTopicPreviewView,
    MonitorTopicUpdateInput,
    MonitorTopicView,
)

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
    409: {"model": ErrorView, "description": "主题状态或版本不允许当前操作"},
    422: {"model": ErrorView, "description": "请求参数校验失败"},
    500: {"model": ErrorView, "description": "服务内部异常"},
}


@router.get(
    "",
    operation_id="listMonitorTopics",
    response_model=PageView[MonitorTopicView],
    status_code=status.HTTP_200_OK,
    summary="列出监控主题",
    description="按当前 owner 列出主题; 默认隐藏已归档主题。",
    responses=_READ_RESPONSES,
)
def list_monitor_topics(
    response: Response,
    service: MonitorTopicServiceDependency,
    identity: AuthenticatedIdentityDependency,
    include_archived: bool = False,
    cursor: UUID | None = None,
    limit: Annotated[int, Query(ge=1, le=50)] = 20,
) -> PageView[MonitorTopicView]:
    items, next_cursor = service.list_topics(
        owner_id=identity.view.user.id,
        include_archived=include_archived,
        cursor=cursor,
        limit=limit,
    )
    response.headers["cache-control"] = "no-store"
    return PageView(items=items, next_cursor=next_cursor)


@router.post(
    "",
    operation_id="createMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_201_CREATED,
    summary="创建监控主题",
    description=(
        "保存本地匹配规则和采集版本; 所选来源须有已应用搜索预设及当前准入策略。"
        "新主题保持暂停, 来源选择会生成停用的搜索调度行。"
    ),
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


@router.post(
    "/preview",
    operation_id="previewMonitorTopic",
    response_model=MonitorTopicPreviewView,
    status_code=status.HTTP_200_OK,
    summary="预览监控主题规则",
    description="只在本地规范化规则并检查标题样本; 不保存主题、不创建任务、不调用来源。",
    responses=_WRITE_RESPONSES,
)
def preview_monitor_topic(
    payload: MonitorTopicPreviewInput,
    response: Response,
    service: MonitorTopicServiceDependency,
    _: CsrfProtectedIdentityDependency,
) -> MonitorTopicPreviewView:
    preview = service.preview_topic(command=payload)
    response.headers["cache-control"] = "no-store"
    return preview


@router.get(
    "/{topic_id}",
    operation_id="getMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_200_OK,
    summary="读取监控主题",
    description="按当前会话 owner 读取主题、当前不可变规则版本、来源选择和运行设置。",
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
    description=(
        "使用 expected_version 防止覆盖并发修改; 规则、来源或主题间隔变化创建采集版本; "
        "名称与报告偏好只更新当前主题, 不改写旧 Job。"
    ),
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


@router.post(
    "/{topic_id}/clone",
    operation_id="cloneMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_201_CREATED,
    summary="复制监控主题",
    description="复制当前规则与运行设置为新主题版本 1; 新主题固定暂停且不复制旧任务。",
    responses=_WRITE_RESPONSES,
)
def clone_monitor_topic(
    topic_id: UUID,
    response: Response,
    service: MonitorTopicServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> MonitorTopicView:
    topic = service.clone_topic(owner_id=identity.view.user.id, topic_id=topic_id)
    response.headers["location"] = f"/api/topics/{topic.id}"
    response.headers["cache-control"] = "no-store"
    return topic


@router.post(
    "/{topic_id}/pause",
    operation_id="pauseMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_200_OK,
    summary="暂停监控主题",
    description="暂停后不允许后续调度; 已运行任务仍需在任务详情单独取消。",
    responses=_WRITE_RESPONSES,
)
def pause_monitor_topic(
    topic_id: UUID,
    response: Response,
    service: MonitorTopicServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> MonitorTopicView:
    topic = service.pause_topic(owner_id=identity.view.user.id, topic_id=topic_id)
    response.headers["cache-control"] = "no-store"
    return topic


@router.post(
    "/{topic_id}/resume",
    operation_id="resumeMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_200_OK,
    summary="恢复监控主题",
    description="仅已准入、启用且预算可用的搜索来源可恢复; 恢复会启用其调度行。",
    responses=_WRITE_RESPONSES,
)
def resume_monitor_topic(
    topic_id: UUID,
    response: Response,
    service: MonitorTopicServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> MonitorTopicView:
    topic = service.resume_topic(owner_id=identity.view.user.id, topic_id=topic_id)
    response.headers["cache-control"] = "no-store"
    return topic


@router.post(
    "/{topic_id}/archive",
    operation_id="archiveMonitorTopic",
    response_model=MonitorTopicView,
    status_code=status.HTTP_200_OK,
    summary="归档监控主题",
    description="归档主题、停用调度并保留规则历史; 不删除资料; 不假报在途任务已取消。",
    responses=_WRITE_RESPONSES,
)
def archive_monitor_topic(
    topic_id: UUID,
    response: Response,
    service: MonitorTopicServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> MonitorTopicView:
    topic = service.archive_topic(owner_id=identity.view.user.id, topic_id=topic_id)
    response.headers["cache-control"] = "no-store"
    return topic
