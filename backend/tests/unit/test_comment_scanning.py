from dataclasses import replace
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

from sqlalchemy.dialects import postgresql

from connections.schemas import SourceEntryPoint
from connections.services import AppliedSourcePreset
from content.services import (
    CommentScanCandidate,
    CommentScanPost,
    CommentScanService,
    _comment_collection_run,
    _rank_comment_posts_for_topic,
    comment_operation_id,
)
from jobs.services import RecentCommentJobTarget, load_recent_comment_job_targets_in_transaction
from monitors.services import ActiveTopicScan, NormalizedMonitorRules
from sources.contracts import SourceCapability


def _topic(*, owner_id: UUID) -> ActiveTopicScan:
    return ActiveTopicScan(
        owner_id=owner_id,
        topic_id=uuid4(),
        topic_version=3,
        rules=NormalizedMonitorRules(match_any=("产品",), match_all=(), exclude=()),
        source_keys=("hackernews",),
    )


def _post(*, owner_id: UUID, like_count: int, title: str = "产品动态") -> CommentScanPost:
    return CommentScanPost(
        owner_id=owner_id,
        content_id=uuid4(),
        source_key="hackernews",
        external_id=f"post-{like_count}-{uuid4()}",
        created_at=datetime(2026, 9, 25, 8, tzinfo=UTC),
        title=title,
        body=None,
        like_count=like_count,
        comment_count=1,
        repost_count=None,
    )


def test_comment_candidates_filter_recent_jobs_before_topic_top_twenty() -> None:
    owner_id = uuid4()
    topic = _topic(owner_id=owner_id)
    posts = tuple(_post(owner_id=owner_id, like_count=value) for value in range(21))
    preset = AppliedSourcePreset(
        source_key="hackernews",
        connection_id=uuid4(),
        connection_version=2,
        capabilities=(SourceCapability.SEARCH, SourceCapability.COMMENTS),
    )
    recent = posts[-1]

    candidates = _rank_comment_posts_for_topic(
        topic=topic,
        posts=posts,
        presets={(owner_id, "hackernews"): preset},
        recent_jobs=frozenset(
            {
                RecentCommentJobTarget(
                    owner_id=owner_id,
                    source_key="hackernews",
                    post_external_id=recent.external_id,
                )
            }
        ),
    )

    assert len(candidates) == 20
    assert recent.content_id not in {item.post.content_id for item in candidates}
    assert [item.post.interaction_score for item in candidates] == list(range(21, 1, -1))


def test_comment_candidate_requires_positive_comments_matching_rules_and_preset() -> None:
    owner_id = uuid4()
    topic = _topic(owner_id=owner_id)
    valid = _post(owner_id=owner_id, like_count=4)
    zero_comments = replace(_post(owner_id=owner_id, like_count=100), comment_count=0)
    unrelated = _post(owner_id=owner_id, like_count=100, title="无关内容")
    preset = AppliedSourcePreset(
        source_key="hackernews",
        connection_id=uuid4(),
        connection_version=1,
        capabilities=(SourceCapability.COMMENTS,),
    )

    without_preset = _rank_comment_posts_for_topic(
        topic=topic,
        posts=(valid,),
        presets={},
        recent_jobs=frozenset(),
    )
    candidates = _rank_comment_posts_for_topic(
        topic=topic,
        posts=(zero_comments, unrelated, valid),
        presets={(owner_id, "hackernews"): preset},
        recent_jobs=frozenset(),
    )

    assert without_preset == ()
    assert tuple(item.post.content_id for item in candidates) == (valid.content_id,)


def test_comment_job_uses_scheduled_entry_point_and_bucket_frozen_scope() -> None:
    owner_id = uuid4()
    topic = _topic(owner_id=owner_id)
    post = _post(owner_id=owner_id, like_count=1)
    preset = AppliedSourcePreset(
        source_key="hackernews",
        connection_id=uuid4(),
        connection_version=4,
        capabilities=(SourceCapability.COMMENTS,),
    )
    bucket = datetime(2026, 9, 25, 6, tzinfo=UTC)

    run = _comment_collection_run(
        CommentScanCandidate(post=post, topic=topic, preset=preset),
        bucket_start=bucket,
    )

    assert run.operation_id == comment_operation_id(post.content_id, bucket)
    assert run.entry_point is SourceEntryPoint.SCHEDULED
    assert run.scheduled_for_at == bucket
    assert run.starts_at == bucket - timedelta(hours=6)
    assert run.ends_at == bucket


def test_recent_post_query_enforces_lifetime_positive_comments_and_latest_rows() -> None:
    class EmptyRows:
        @staticmethod
        def all() -> list[object]:
            return []

    class CapturingSession:
        statement: object | None = None

        def execute(self, statement: object) -> EmptyRows:
            self.statement = statement
            return EmptyRows()

    session = CapturingSession()
    now = datetime(2026, 9, 25, 12, tzinfo=UTC)

    assert (
        CommentScanService(session)._load_recent_posts(  # type: ignore[arg-type]
            owners={uuid4()},
            source_keys={"hackernews"},
            since=now - timedelta(hours=24),
            until=now,
        )
        == ()
    )
    assert session.statement is not None
    sql = str(session.statement.compile(dialect=postgresql.dialect()))  # type: ignore[attr-defined]
    assert "content_records.created_at >=" in sql
    assert "content_records.created_at <=" in sql
    assert "comment_count >" in sql
    assert sql.count("row_number() OVER") == 2


def test_recent_comment_job_query_uses_job_creation_time_and_post_identity() -> None:
    class EmptyRows:
        @staticmethod
        def all() -> list[object]:
            return []

    class CapturingSession:
        statement: object | None = None

        @staticmethod
        def in_transaction() -> bool:
            return True

        def execute(self, statement: object) -> EmptyRows:
            self.statement = statement
            return EmptyRows()

    session = CapturingSession()

    assert (
        load_recent_comment_job_targets_in_transaction(  # type: ignore[arg-type]
            session,
            since=datetime(2026, 9, 25, 6, tzinfo=UTC),
        )
        == frozenset()
    )
    assert session.statement is not None
    sql = str(session.statement.compile(dialect=postgresql.dialect()))  # type: ignore[attr-defined]
    assert "jobs.kind =" in sql
    assert "jobs.created_at >" in sql
    assert "jobs.scope ->>" in sql
