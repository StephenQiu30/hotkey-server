from __future__ import annotations

from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from pydantic import ValidationError
from sqlalchemy.orm import sessionmaker

from connections.schemas import SourceConnectionConfig, SourceEntryPoint
from content.comments import (
    CommentPageCommitService,
    CommentTreeLimiter,
    build_comment_job_acceptance,
    comment_target_hash,
)
from content.comments_execution import (
    UnsupportedCommentsSourceError,
    build_comments_adapter_factory,
)
from content.schemas import CommentCollectionRunInput
from core.config import Settings
from sources.adapters.hackernews import HackerNewsAdapter
from sources.contracts import SourceComment
from worker.app import _registered_job_handlers


def _comment(
    identifier: str,
    *,
    parent: str | None = None,
    post_external_id: str = "42",
) -> SourceComment:
    return SourceComment(
        source_key="hackernews",
        external_id=identifier,
        post_external_id=post_external_id,
        parent_comment_external_id=parent,
        author_external_id="alice",
        author_name="Alice",
        published_at=datetime(2026, 9, 25, tzinfo=UTC),
        text=f"comment {identifier}",
        like_count=1,
        canonical_url=f"https://news.ycombinator.com/item?id={identifier}",
    )


def _run(**changes: object) -> CommentCollectionRunInput:
    starts_at = datetime(2026, 9, 25, tzinfo=UTC)
    values: dict[str, object] = {
        "operation_id": uuid4(),
        "configuration_ref": "topic:fixture",
        "configuration_version": 3,
        "source_key": "hackernews",
        "connection_id": uuid4(),
        "connection_version": 2,
        "post_external_id": "42",
        "entry_point": SourceEntryPoint.SCHEDULED,
        "starts_at": starts_at,
        "ends_at": starts_at + timedelta(hours=6),
        "scheduled_for_at": starts_at,
    }
    values.update(changes)
    return CommentCollectionRunInput.model_validate(values)


def test_comment_run_freezes_bounded_post_scope() -> None:
    run = _run()
    command = build_comment_job_acceptance(run)

    assert run.post_external_id == "42"
    assert run.page_size == 100
    assert run.max_pages == 10
    assert run.max_requests == 10
    assert run.max_seconds == 90
    assert run.scan_kind.value == "refresh"
    assert command.operation_id == run.operation_id
    assert command.kind == "source.comments"
    assert command.observation.source_capability.value == "comments"
    assert command.scope["post_external_id"] == "42"
    assert command.scope["first_level_limit"] == 200
    assert command.scope["replies_per_thread_limit"] == 20
    assert command.scope["target_hash"] == comment_target_hash("topic:fixture", "42").hex()


@pytest.mark.parametrize(
    "changes",
    [
        {"post_external_id": " bad"},
        {"page_size": 101},
        {"max_pages": 33},
        {"max_requests": 101},
        {"max_seconds": 91},
        {"ends_at": datetime(2026, 9, 25, tzinfo=UTC) + timedelta(days=2)},
        {"starts_at": datetime(2026, 9, 25)},
        {"scheduled_for_at": datetime(2026, 9, 25)},
    ],
)
def test_comment_run_rejects_unbounded_or_ambiguous_scope(changes: dict[str, object]) -> None:
    with pytest.raises(ValidationError):
        _run(**changes)


def test_tree_limiter_keeps_200_roots_and_20_replies_per_root() -> None:
    limiter = CommentTreeLimiter(first_level_limit=200, replies_per_thread_limit=20)
    first_page = [_comment("root-1")]
    first_page.extend(_comment(f"reply-{index}", parent="root-1") for index in range(1, 26))
    admitted_first, filtered_first = limiter.admit(first_page)

    remaining_roots = [_comment(f"root-{index}") for index in range(2, 202)]
    admitted_second, filtered_second = limiter.admit(remaining_roots)

    assert [item.external_id for item in admitted_first] == [
        "root-1",
        *(f"reply-{index}" for index in range(1, 21)),
    ]
    assert filtered_first == 5
    assert len(admitted_second) == 199
    assert admitted_second[-1].external_id == "root-200"
    assert filtered_second == 1


def test_tree_limiter_preserves_parent_chain_and_counts_duplicate_as_filtered() -> None:
    limiter = CommentTreeLimiter(first_level_limit=200, replies_per_thread_limit=20)
    root = _comment("root")
    child = _comment("child", parent="root")
    grandchild = _comment("grandchild", parent="child")

    admitted, filtered = limiter.admit((root, child, grandchild, child))

    assert admitted == (root, child, grandchild)
    assert filtered == 1


def test_tree_limiter_treats_a_missing_parent_as_a_bounded_placeholder_root() -> None:
    limiter = CommentTreeLimiter(first_level_limit=200, replies_per_thread_limit=20)
    replies = tuple(_comment(f"reply-{index}", parent="deleted") for index in range(21))

    admitted, filtered = limiter.admit(replies)

    assert len(admitted) == 20
    assert all(item.parent_comment_external_id == "deleted" for item in admitted)
    assert filtered == 1


def test_comment_payload_maps_thread_author_metric_and_text_fields() -> None:
    payload = CommentPageCommitService._payload(_comment("child", parent="root"))

    assert payload == {
        "object_type": "comment",
        "external_id": "child",
        "post_external_id": "42",
        "parent_comment_external_id": "root",
        "published_at": "2026-09-25T00:00:00+00:00",
        "like_count": 1,
        "author_external_id": "alice",
        "author_name": "Alice",
        "canonical_url": "https://news.ycombinator.com/item?id=child",
        "body": "comment child",
        "text_scope": "full",
        "text_origin": "source",
    }


def test_hackernews_comments_factory_uses_connection_allowed_hosts() -> None:
    factory = build_comments_adapter_factory(
        "hackernews",
        SourceConnectionConfig(
            base_url="https://hn.algolia.com/api/v1",
            allowed_hosts=("hn.algolia.com",),
        ),
    )
    adapter = factory(lambda _attempt: True, lambda: False, 4, 30.0)

    assert isinstance(adapter, HackerNewsAdapter)
    assert adapter.source_key == "hackernews"
    assert adapter._allowed_hosts == frozenset({"hn.algolia.com"})


def test_unknown_comments_source_is_explicitly_rejected() -> None:
    with pytest.raises(UnsupportedCommentsSourceError):
        build_comments_adapter_factory(
            "unknown_source",
            SourceConnectionConfig(allowed_hosts=("example.com",)),
        )


def test_worker_registers_comments_handler() -> None:
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")

    handlers = _registered_job_handlers(sessionmaker(), settings)

    assert "source.comments" in handlers
