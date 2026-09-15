from typing import Annotated

from fastapi import APIRouter, Query

from api.dependencies import Authenticated, Contents
from contents.schemas import InboxPage

router = APIRouter(prefix="/api/v1")


@router.get(
    "/contents",
    response_model=InboxPage,
    tags=["contents"],
    operation_id="listInboxContents",
)
def inbox_contents(
    service: Contents,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    cursor: Annotated[str | None, Query(max_length=100)] = None,
) -> InboxPage:
    return service.inbox(limit, cursor)
