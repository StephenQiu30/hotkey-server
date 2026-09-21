from __future__ import annotations

import secrets
from datetime import datetime
from typing import Annotated, Any

from fastapi import APIRouter, Header, Response, status

from api.dependencies import (
    AuthenticatedIdentityDependency,
    CsrfProtectedIdentityDependency,
    IdentityServiceDependency,
)
from core.errors import ApplicationError
from core.schemas import ErrorView
from identity.schemas import (
    IdentityCredentialsInput,
    IdentitySessionView,
    IdentityWorkspaceView,
)

router = APIRouter(prefix="/identity", tags=["identity"])

_COMMON_ERROR_RESPONSES: dict[int | str, dict[str, Any]] = {
    422: {"model": ErrorView},
    500: {"model": ErrorView},
}


def _require_public_mutation(csrf_header: str | None) -> None:
    if csrf_header is None or not secrets.compare_digest(csrf_header, "1"):
        raise ApplicationError("csrf_invalid")


def _set_session_cookies(
    response: Response,
    service: IdentityServiceDependency,
    *,
    session_token: str,
    csrf_token: str,
    expires_at: datetime,
) -> None:
    response.set_cookie(
        "hotkey_session",
        session_token,
        httponly=True,
        secure=service.cookie_secure,
        samesite="strict",
        path="/",
        max_age=service.session_ttl_seconds,
        expires=expires_at,
    )
    response.set_cookie(
        "hotkey_csrf",
        csrf_token,
        httponly=False,
        secure=service.cookie_secure,
        samesite="strict",
        path="/",
        max_age=service.session_ttl_seconds,
        expires=expires_at,
    )
    response.headers["cache-control"] = "no-store"


@router.post(
    "/initialize",
    operation_id="initializeOwner",
    response_model=IdentitySessionView,
    status_code=status.HTTP_201_CREATED,
    summary="初始化首个使用者",
    description="需要部署者配置的 bootstrap 凭据。成功后建立唯一 owner 与首个会话。",
    responses={
        403: {"model": ErrorView},
        409: {"model": ErrorView},
        **_COMMON_ERROR_RESPONSES,
    },
)
def initialize_owner(
    payload: IdentityCredentialsInput,
    response: Response,
    service: IdentityServiceDependency,
    bootstrap_token: Annotated[
        str | None,
        Header(alias="X-HotKey-Bootstrap-Token"),
    ] = None,
    csrf_header: Annotated[str | None, Header(alias="X-HotKey-CSRF")] = None,
) -> IdentitySessionView:
    _require_public_mutation(csrf_header)
    created = service.initialize_owner(
        username=payload.username,
        password=payload.password.get_secret_value(),
        bootstrap_token=bootstrap_token,
    )
    _set_session_cookies(
        response,
        service,
        session_token=created.session_token,
        csrf_token=created.csrf_token,
        expires_at=created.view.expires_at,
    )
    return created.view


@router.post(
    "/sessions",
    operation_id="createIdentitySession",
    response_model=IdentitySessionView,
    status_code=status.HTTP_200_OK,
    summary="登录并建立会话",
    description="验证 owner 凭据并设置服务端可撤销的不透明会话 Cookie。",
    responses={
        401: {"model": ErrorView},
        403: {"model": ErrorView},
        **_COMMON_ERROR_RESPONSES,
    },
)
def create_identity_session(
    payload: IdentityCredentialsInput,
    response: Response,
    service: IdentityServiceDependency,
    csrf_header: Annotated[str | None, Header(alias="X-HotKey-CSRF")] = None,
) -> IdentitySessionView:
    _require_public_mutation(csrf_header)
    created = service.login(
        username=payload.username,
        password=payload.password.get_secret_value(),
    )
    _set_session_cookies(
        response,
        service,
        session_token=created.session_token,
        csrf_token=created.csrf_token,
        expires_at=created.view.expires_at,
    )
    return created.view


@router.get(
    "/session",
    operation_id="getIdentitySession",
    response_model=IdentitySessionView,
    status_code=status.HTTP_200_OK,
    summary="读取当前会话",
    description="需要有效的 SessionCookie。只返回非敏感的使用者和过期时间。",
    responses={
        401: {"model": ErrorView},
        422: {"model": ErrorView},
        500: {"model": ErrorView},
    },
)
def get_identity_session(identity: AuthenticatedIdentityDependency) -> IdentitySessionView:
    return identity.view


@router.get(
    "/workspace",
    operation_id="getIdentityWorkspace",
    response_model=IdentityWorkspaceView,
    status_code=status.HTTP_200_OK,
    summary="读取当前私有工作区",
    description="工作区 owner 只从有效会话派生。接口不接受客户端提供归属标识。",
    responses={
        401: {"model": ErrorView},
        404: {"model": ErrorView},
        422: {"model": ErrorView},
        500: {"model": ErrorView},
    },
)
def get_identity_workspace(
    response: Response,
    service: IdentityServiceDependency,
    identity: AuthenticatedIdentityDependency,
) -> IdentityWorkspaceView:
    response.headers["cache-control"] = "no-store"
    return service.get_workspace(identity)


@router.delete(
    "/session",
    operation_id="deleteIdentitySession",
    response_model=None,
    status_code=status.HTTP_204_NO_CONTENT,
    summary="注销当前会话",
    description="需要有效的 SessionCookie 与匹配的 X-HotKey-CSRF 请求头。",
    responses={
        401: {"model": ErrorView},
        403: {"model": ErrorView},
        422: {"model": ErrorView},
        500: {"model": ErrorView},
    },
)
def delete_identity_session(
    response: Response,
    service: IdentityServiceDependency,
    identity: CsrfProtectedIdentityDependency,
) -> None:
    service.logout(identity.session_id)
    response.delete_cookie(
        "hotkey_session",
        secure=service.cookie_secure,
        httponly=True,
        samesite="strict",
        path="/",
    )
    response.delete_cookie(
        "hotkey_csrf",
        secure=service.cookie_secure,
        httponly=False,
        samesite="strict",
        path="/",
    )
    response.headers["cache-control"] = "no-store"
