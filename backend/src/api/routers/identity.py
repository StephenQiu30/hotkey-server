from fastapi import APIRouter, Request, Response

from api.dependencies import Authenticated, Identity
from api.responses import (
    PUBLIC_WRITE_ERROR_CODES,
    READ_ERROR_CODES,
    WRITE_ERROR_CODES,
    error_responses,
)
from identity.schemas import LoginInput, Principal

router = APIRouter()


@router.post(
    "/session",
    response_model=Principal,
    responses=error_responses(*PUBLIC_WRITE_ERROR_CODES, 401, 429),
    tags=["identity"],
    operation_id="login",
)
def login(data: LoginInput, response: Response, request: Request, service: Identity) -> Principal:
    token, csrf = service.login(data.username, data.password.get_secret_value())
    secure = request.app.state.settings.cookie_secure
    response.set_cookie(
        "hk_session", token, max_age=43200, httponly=True, secure=secure, samesite="lax", path="/"
    )
    response.set_cookie("hk_csrf", csrf, max_age=43200, secure=secure, samesite="lax", path="/")
    return Principal(username=data.username)


@router.get(
    "/session",
    response_model=Principal,
    responses=error_responses(*READ_ERROR_CODES),
    tags=["identity"],
    operation_id="getSession",
)
def me(owner: Authenticated) -> Principal:
    return owner


@router.delete(
    "/session",
    status_code=204,
    responses=error_responses(*WRITE_ERROR_CODES),
    tags=["identity"],
    operation_id="logout",
)
def logout(request: Request, response: Response, service: Identity, owner: Authenticated) -> None:
    service.logout(request.cookies["hk_session"])
    response.delete_cookie("hk_session", path="/")
    response.delete_cookie("hk_csrf", path="/")
