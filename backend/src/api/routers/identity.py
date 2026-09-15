from fastapi import APIRouter, Request, Response

from api.dependencies import Authenticated, Identity
from identity.schemas import LoginInput, Principal

router = APIRouter(prefix="/api/v1")


@router.post("/session", response_model=Principal, tags=["identity"], operation_id="login")
def login(data: LoginInput, response: Response, request: Request, service: Identity) -> Principal:
    token, csrf = service.login(data.username, data.password.get_secret_value())
    secure = request.app.state.settings.cookie_secure
    response.set_cookie(
        "hk_session", token, max_age=43200, httponly=True, secure=secure, samesite="lax", path="/"
    )
    response.set_cookie("hk_csrf", csrf, max_age=43200, secure=secure, samesite="lax", path="/")
    return Principal(username=data.username)


@router.get("/session", response_model=Principal, tags=["identity"], operation_id="getSession")
def me(owner: Authenticated) -> Principal:
    return owner


@router.delete("/session", status_code=204, tags=["identity"], operation_id="logout")
def logout(request: Request, response: Response, service: Identity, owner: Authenticated) -> None:
    service.logout(request.cookies["hk_session"])
    response.delete_cookie("hk_session", path="/")
    response.delete_cookie("hk_csrf", path="/")
