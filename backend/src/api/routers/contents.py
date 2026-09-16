from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query
from pydantic import AwareDatetime

from api.dependencies import Authenticated, Contents, Knowledge
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from contents.schemas import ContentWithdrawalInput, ContentWithdrawalView, InboxPage
from monitors.schemas import MonitorMatchReviewState
from sources.schemas import SourceName

router = APIRouter()


@router.get(
    "/contents",
    response_model=InboxPage,
    responses=error_responses(*READ_ERROR_CODES),
    tags=["contents"],
    operation_id="listInboxContents",
)
def inbox_contents(
    service: Contents,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    cursor: Annotated[str | None, Query(max_length=100)] = None,
    monitor_id: UUID | None = None,
    source: SourceName | None = None,
    review_state: MonitorMatchReviewState | None = None,
    discovered_since: Annotated[AwareDatetime | None, Query()] = None,
) -> InboxPage:
    return service.inbox(
        limit,
        cursor,
        monitor_id=monitor_id,
        source=source,
        review_state=review_state,
        discovered_since=discovered_since,
    )


@router.post(
    "/contents/{identity}/withdraw",
    response_model=ContentWithdrawalView,
    responses=error_responses(*WRITE_ERROR_CODES, 404),
    tags=["contents"],
    operation_id="withdrawContent",
)
def withdraw_content(
    identity: UUID,
    data: ContentWithdrawalInput,
    service: Knowledge,
    owner: Authenticated,
) -> ContentWithdrawalView:
    return service.withdraw_content(identity, data)
