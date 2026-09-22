from __future__ import annotations

from datetime import UTC, datetime

import pytest
from pydantic import ValidationError

from sources.contracts import (
    SOCIAL_CAPABILITIES,
    AuthorPostsRequest,
    CommentsRequest,
    RepliesRequest,
    SearchRequest,
    SourceAdapter,
    SourceCapability,
    SourceComment,
    SourcePage,
    SourcePageState,
    SourcePost,
    SourceRequest,
    SourceStopReason,
)

OBSERVED_AT = datetime(2026, 9, 21, 12, tzinfo=UTC)


def _post(**changes: object) -> SourcePost:
    values: dict[str, object] = {
        "source_key": "source-a",
        "external_id": "post-1",
        "author_external_id": "author-1",
        "published_at": OBSERVED_AT,
        "text": "public post",
        "like_count": None,
        "comment_count": 2,
        "repost_count": 1,
    }
    values.update(changes)
    return SourcePost.model_validate(values)


def _comment(**changes: object) -> SourceComment:
    values: dict[str, object] = {
        "source_key": "source-a",
        "external_id": "comment-2",
        "post_external_id": "post-1",
        "parent_comment_external_id": "comment-1",
        "author_external_id": None,
        "published_at": OBSERVED_AT,
        "text": "public reply",
        "like_count": None,
    }
    values.update(changes)
    return SourceComment.model_validate(values)


def test_capability_requests_keep_distinct_targets_and_opaque_paging() -> None:
    search = SearchRequest(
        source_key="source-a",
        query="topic",
        page_size=20,
        page_token="opaque_page-2",
        watermark="opaque_watermark-1",
    )
    author_posts = AuthorPostsRequest(
        source_key="source-a",
        author_external_id="author-1",
        page_size=20,
    )
    comments = CommentsRequest(
        source_key="source-a",
        post_external_id="post-1",
        page_size=20,
    )
    replies = RepliesRequest(
        source_key="source-a",
        comment_external_id="comment-1",
        page_size=20,
    )

    assert [
        search.capability,
        author_posts.capability,
        comments.capability,
        replies.capability,
    ] == list(SOCIAL_CAPABILITIES)

    with pytest.raises(ValidationError):
        SearchRequest(source_key="source-a", query="topic", page_size=0)
    with pytest.raises(ValidationError):
        SearchRequest(
            source_key="source-a",
            query="topic",
            page_size=20,
            page_token="not url safe",
        )
    with pytest.raises(ValidationError):
        SearchRequest(
            source_key="source-a",
            query="topic",
            page_size=20,
            provider_secret="must-not-enter-contract",
        )


def test_page_content_cannot_enter_the_social_paging_contract() -> None:
    with pytest.raises(ValidationError):
        SourcePage(
            source_key="source-a",
            capability=SourceCapability.PAGE_CONTENT,
            state=SourcePageState.STOPPED,
            items=(),
            next_page_token=None,
            watermark=None,
            stop_reason=SourceStopReason.UNSUPPORTED,
            observed_at=OBSERVED_AT,
        )


def test_missing_metrics_and_parent_relationship_must_be_explicit() -> None:
    post = _post()
    comment = _comment()

    assert post.like_count is None
    assert comment.parent_comment_external_id == "comment-1"
    assert comment.like_count is None

    post_input = post.model_dump()
    del post_input["like_count"]
    with pytest.raises(ValidationError):
        SourcePost.model_validate(post_input)

    comment_input = comment.model_dump()
    del comment_input["parent_comment_external_id"]
    with pytest.raises(ValidationError):
        SourceComment.model_validate(comment_input)


def test_normalized_objects_reject_silent_value_coercion() -> None:
    with pytest.raises(ValidationError):
        _post(like_count=False)
    with pytest.raises(ValidationError):
        _post(comment_count=-1)
    with pytest.raises(ValidationError):
        _post(external_id=" post-1 ")
    with pytest.raises(ValidationError):
        _comment(published_at=datetime(2026, 9, 21, 12))


def test_page_state_distinguishes_more_complete_empty_partial_and_stopped() -> None:
    post = _post()
    comment = _comment()

    more = SourcePage(
        source_key="source-a",
        capability=SourceCapability.SEARCH,
        state=SourcePageState.MORE,
        items=(),
        next_page_token="opaque_page-2",
        watermark="opaque_watermark-1",
        stop_reason=None,
        observed_at=OBSERVED_AT,
    )
    complete = SourcePage(
        source_key="source-a",
        capability=SourceCapability.SEARCH,
        state=SourcePageState.COMPLETE,
        items=(post,),
        next_page_token=None,
        watermark="opaque_watermark-2",
        stop_reason=SourceStopReason.END_OF_RESULTS,
        observed_at=OBSERVED_AT,
    )
    empty = SourcePage(
        source_key="source-a",
        capability=SourceCapability.COMMENTS,
        state=SourcePageState.EMPTY,
        items=(),
        next_page_token=None,
        watermark=None,
        stop_reason=SourceStopReason.SOURCE_EMPTY,
        observed_at=OBSERVED_AT,
    )
    partial = SourcePage(
        source_key="source-a",
        capability=SourceCapability.SEARCH,
        state=SourcePageState.PARTIAL,
        items=(post,),
        next_page_token="opaque_resume-1",
        watermark=None,
        stop_reason=SourceStopReason.RATE_LIMITED,
        observed_at=OBSERVED_AT,
    )
    stopped = SourcePage(
        source_key="source-a",
        capability=SourceCapability.REPLIES,
        state=SourcePageState.STOPPED,
        items=(),
        next_page_token=None,
        watermark=None,
        stop_reason=SourceStopReason.ACCESS_DENIED,
        observed_at=OBSERVED_AT,
    )

    assert more.next_page_token == "opaque_page-2"
    assert complete.stop_reason is SourceStopReason.END_OF_RESULTS
    assert empty.stop_reason is SourceStopReason.SOURCE_EMPTY
    assert partial.items == (post,)
    assert stopped.stop_reason is SourceStopReason.ACCESS_DENIED

    invalid_pages = (
        {"state": SourcePageState.MORE, "items": (), "stop_reason": None},
        {
            "state": SourcePageState.COMPLETE,
            "items": (),
            "stop_reason": SourceStopReason.END_OF_RESULTS,
        },
        {
            "state": SourcePageState.EMPTY,
            "items": (post,),
            "stop_reason": SourceStopReason.SOURCE_EMPTY,
        },
        {
            "state": SourcePageState.PARTIAL,
            "items": (),
            "stop_reason": SourceStopReason.RATE_LIMITED,
        },
        {
            "state": SourcePageState.STOPPED,
            "items": (post,),
            "stop_reason": SourceStopReason.ACCESS_DENIED,
        },
    )
    for values in invalid_pages:
        with pytest.raises(ValidationError):
            SourcePage.model_validate(
                {
                    "source_key": "source-a",
                    "capability": SourceCapability.SEARCH,
                    "next_page_token": None,
                    "watermark": None,
                    "observed_at": OBSERVED_AT,
                    **values,
                }
            )

    with pytest.raises(ValidationError, match="source and capability"):
        SourcePage(
            source_key="source-a",
            capability=SourceCapability.REPLIES,
            state=SourcePageState.COMPLETE,
            items=(post,),
            next_page_token=None,
            watermark=None,
            stop_reason=SourceStopReason.END_OF_RESULTS,
            observed_at=OBSERVED_AT,
        )
    with pytest.raises(ValidationError, match="source and capability"):
        SourcePage(
            source_key="source-b",
            capability=SourceCapability.COMMENTS,
            state=SourcePageState.COMPLETE,
            items=(comment,),
            next_page_token=None,
            watermark=None,
            stop_reason=SourceStopReason.END_OF_RESULTS,
            observed_at=OBSERVED_AT,
        )


class _StaticAdapter:
    source_key = "source-a"
    capabilities = frozenset({SourceCapability.SEARCH})

    def fetch_page(self, request: SourceRequest) -> SourcePage:
        assert request.capability in self.capabilities
        return SourcePage(
            source_key=self.source_key,
            capability=request.capability,
            state=SourcePageState.EMPTY,
            items=(),
            next_page_token=None,
            watermark=None,
            stop_reason=SourceStopReason.SOURCE_EMPTY,
            observed_at=OBSERVED_AT,
        )


def _fetch(adapter: SourceAdapter, request: SourceRequest) -> SourcePage:
    return adapter.fetch_page(request)


def test_adapter_uses_structural_contract_without_framework_inheritance() -> None:
    result = _fetch(
        _StaticAdapter(),
        SearchRequest(source_key="source-a", query="topic", page_size=20),
    )

    assert result.state is SourcePageState.EMPTY
