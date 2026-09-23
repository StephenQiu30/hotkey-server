from __future__ import annotations

from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, Query, Response, status

from api.dependencies import (
    AuthenticatedIdentityDependency,
    CsrfProtectedIdentityDependency,
    JobServiceDependency,
    WebPageCollectionServiceDependency,
)
from core.schemas import ErrorView, JobAcceptedView, PageView
from jobs.schemas import (
    JobContinuousFailureIssueView,
    JobHistoryItemView,
    JobStatusView,
    WebPageCollectionJobInput,
)

router = APIRouter(prefix="/jobs", tags=["采集任务"])

_READ_ERROR_RESPONSES: dict[int | str, dict[str, Any]] = {
    401: {"model": ErrorView, "description": "会话无效或已过期"},
    404: {"model": ErrorView, "description": "任务不存在或不可访问"},
    422: {"model": ErrorView, "description": "请求参数校验失败"},
    500: {"model": ErrorView, "description": "服务内部异常"},
}


@router.post(
    "",
    operation_id="createCollectionJob",
    response_model=JobAcceptedView,
    status_code=status.HTTP_202_ACCEPTED,
    summary="提交采集任务",
    description="任务与 Outbox 持久提交后才返回受理, 相同操作标识复用原任务。",
    responses={
        401: {"model": ErrorView, "description": "会话无效或已过期"},
        403: {"model": ErrorView, "description": "请求安全校验失败"},
        409: {"model": ErrorView, "description": "操作标识与原任务意图冲突"},
        422: {"model": ErrorView, "description": "请求参数校验失败"},
        500: {"model": ErrorView, "description": "服务内部异常"},
    },
)
def create_collection_job(
    payload: WebPageCollectionJobInput,
    response: Response,
    webpage_service: WebPageCollectionServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> JobAcceptedView:
    job = webpage_service.accept_job(
        owner_id=identity.view.user.id,
        operation_id=payload.operation_id,
        target_url=payload.url,
    )
    response.headers["location"] = f"/api/jobs/{job.id}"
    response.headers["cache-control"] = "no-store"
    return JobAcceptedView(job_id=job.id, status="queued")


@router.get(
    "",
    operation_id="listCollectionJobs",
    response_model=PageView[JobHistoryItemView],
    status_code=status.HTTP_200_OK,
    summary="列出采集任务",
    description="按当前会话 owner 稳定分页读取任务摘要; 不暴露 scope、租约、操作标识或内部消息。",
    responses=_READ_ERROR_RESPONSES,
)
def list_collection_jobs(
    response: Response,
    service: JobServiceDependency,
    identity: AuthenticatedIdentityDependency,
    cursor: UUID | None = None,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
) -> PageView[JobHistoryItemView]:
    items, next_cursor = service.list_history(
        owner_id=identity.view.user.id,
        cursor=cursor,
        limit=limit,
    )
    response.headers["cache-control"] = "no-store"
    return PageView(items=items, next_cursor=next_cursor)


@router.get(
    "/issues",
    operation_id="listContinuousFailureIssues",
    response_model=list[JobContinuousFailureIssueView],
    status_code=status.HTTP_200_OK,
    summary="列出连续失败问题",
    description="按当前会话 owner 返回连续三次失败的来源能力摘要供查看最近失败任务处理动作",
    responses={
        401: {"model": ErrorView, "description": "会话无效或已过期"},
        500: {"model": ErrorView, "description": "服务内部异常"},
    },
)
def list_continuous_failure_issues(
    response: Response,
    service: JobServiceDependency,
    identity: AuthenticatedIdentityDependency,
) -> list[JobContinuousFailureIssueView]:
    issues = service.list_continuous_failure_issues(owner_id=identity.view.user.id)
    response.headers["cache-control"] = "no-store"
    return list(issues)


@router.get(
    "/{job_id}",
    operation_id="getCollectionJob",
    response_model=JobStatusView,
    status_code=status.HTTP_200_OK,
    summary="读取采集任务状态",
    description="按当前会话 owner 读取持久任务, 不会暴露 scope、租约或内部消息。",
    responses=_READ_ERROR_RESPONSES,
)
def get_collection_job(
    job_id: UUID,
    response: Response,
    service: JobServiceDependency,
    identity: AuthenticatedIdentityDependency,
) -> JobStatusView:
    job = service.get_status(owner_id=identity.view.user.id, job_id=job_id)
    response.headers["cache-control"] = "no-store"
    return job


@router.post(
    "/{job_id}/cancel",
    operation_id="cancelCollectionJob",
    response_model=JobStatusView,
    status_code=status.HTTP_200_OK,
    summary="取消采集任务",
    description="排队任务立即取消; 运行任务持久化取消意图并等待在途响应收尾。",
    responses={
        401: {"model": ErrorView, "description": "会话无效或已过期"},
        403: {"model": ErrorView, "description": "请求安全校验失败"},
        404: {"model": ErrorView, "description": "任务不存在或不可访问"},
        409: {"model": ErrorView, "description": "任务当前状态不可取消"},
        422: {"model": ErrorView, "description": "请求参数校验失败"},
        500: {"model": ErrorView, "description": "服务内部异常"},
    },
)
def cancel_collection_job(
    job_id: UUID,
    response: Response,
    service: JobServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> JobStatusView:
    job = service.request_cancel(owner_id=identity.view.user.id, job_id=job_id)
    response.headers["cache-control"] = "no-store"
    return job


@router.post(
    "/{job_id}/retry",
    operation_id="retryCollectionJob",
    response_model=JobStatusView,
    status_code=status.HTTP_202_ACCEPTED,
    summary="重试采集任务",
    description="恢复同一任务及其检查点; 重复点击已排队任务不会重复派发。",
    responses={
        401: {"model": ErrorView, "description": "会话无效或已过期"},
        403: {"model": ErrorView, "description": "请求安全校验失败"},
        404: {"model": ErrorView, "description": "任务不存在或不可访问"},
        409: {"model": ErrorView, "description": "任务当前状态不可重试"},
        422: {"model": ErrorView, "description": "请求参数校验失败"},
        500: {"model": ErrorView, "description": "服务内部异常"},
    },
)
def retry_collection_job(
    job_id: UUID,
    response: Response,
    service: JobServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> JobStatusView:
    job = service.request_retry(owner_id=identity.view.user.id, job_id=job_id)
    response.headers["location"] = f"/api/jobs/{job.id}"
    response.headers["cache-control"] = "no-store"
    return job
