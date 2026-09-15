"""Collection page transaction foundation

Revision ID: 0005_collection_page
Revises: 0004_monitor_versions
Create Date: 2026-09-15 20:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0005_collection_page"
down_revision: str | Sequence[str] | None = "0004_monitor_versions"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "collection_runs",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("monitor_version_id", sa.Uuid(), nullable=False),
        sa.Column("source", sa.String(length=32), nullable=False),
        sa.Column("operation", sa.String(length=32), nullable=False),
        sa.Column("query_variant", sa.String(length=100), nullable=False),
        sa.Column("idempotency_key", sa.String(length=128), nullable=False),
        sa.Column("policy_version", sa.String(length=64), nullable=False),
        sa.Column("state", sa.String(length=16), nullable=False),
        sa.Column("outcome", sa.String(length=16), nullable=True),
        sa.Column("fencing_token", sa.Integer(), nullable=False),
        sa.Column("window_since", sa.DateTime(timezone=True), nullable=False),
        sa.Column("window_until", sa.DateTime(timezone=True), nullable=False),
        sa.Column("pages_count", sa.Integer(), nullable=False),
        sa.Column("items_count", sa.Integer(), nullable=False),
        sa.Column("bytes_count", sa.Integer(), nullable=False),
        sa.Column("stop_reason", sa.String(length=80), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("completed_at", sa.DateTime(timezone=True), nullable=True),
        sa.CheckConstraint(
            "fencing_token >= 0 AND pages_count >= 0 AND items_count >= 0 AND bytes_count >= 0",
            name="ck_collection_runs_counts",
        ),
        sa.CheckConstraint(
            "outcome IS NULL OR outcome IN ('ok', 'empty', 'partial', 'failed')",
            name="ck_collection_runs_outcome",
        ),
        sa.CheckConstraint(
            "state IN ('queued', 'running', 'completed', 'failed', 'cancelled')",
            name="ck_collection_runs_state",
        ),
        sa.CheckConstraint("window_since < window_until", name="ck_collection_runs_window"),
        sa.ForeignKeyConstraint(
            ["monitor_version_id"], ["monitor_versions.id"], ondelete="CASCADE"
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("idempotency_key", name="uq_collection_runs_idempotency_key"),
    )
    op.create_index(
        op.f("ix_collection_runs_monitor_version_id"),
        "collection_runs",
        ["monitor_version_id"],
        unique=False,
    )
    op.create_table(
        "raw_pages",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("run_id", sa.Uuid(), nullable=False),
        sa.Column("source", sa.String(length=32), nullable=False),
        sa.Column("operation", sa.String(length=32), nullable=False),
        sa.Column("request_fingerprint", sa.String(length=64), nullable=False),
        sa.Column("bucket", sa.String(length=255), nullable=False),
        sa.Column("object_key", sa.String(length=1024), nullable=False),
        sa.Column("payload_sha256", sa.String(length=64), nullable=False),
        sa.Column("object_sha256", sa.String(length=64), nullable=False),
        sa.Column("response_bytes", sa.Integer(), nullable=False),
        sa.Column("object_bytes", sa.Integer(), nullable=False),
        sa.Column("media_type", sa.String(length=64), nullable=False),
        sa.Column("observed_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("retention_until", sa.DateTime(timezone=True), nullable=False),
        sa.Column("policy_version", sa.String(length=64), nullable=False),
        sa.CheckConstraint("response_bytes >= 0 AND object_bytes >= 0", name="ck_raw_pages_bytes"),
        sa.CheckConstraint("retention_until > observed_at", name="ck_raw_pages_retention"),
        sa.ForeignKeyConstraint(["run_id"], ["collection_runs.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("object_key", name="uq_raw_pages_object_key"),
        sa.UniqueConstraint(
            "run_id",
            "request_fingerprint",
            "payload_sha256",
            name="uq_raw_pages_response",
        ),
    )
    op.create_index(op.f("ix_raw_pages_run_id"), "raw_pages", ["run_id"], unique=False)
    op.create_table(
        "contents",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("source", sa.String(length=32), nullable=False),
        sa.Column("provider_namespace", sa.String(length=100), nullable=False),
        sa.Column("external_id", sa.String(length=1024), nullable=False),
        sa.Column("kind", sa.String(length=16), nullable=False),
        sa.Column("canonical_url", sa.String(length=2048), nullable=True),
        sa.Column("author_ref", sa.String(length=1024), nullable=False),
        sa.Column("root_external_id", sa.String(length=1024), nullable=False),
        sa.Column("parent_external_id", sa.String(length=1024), nullable=True),
        sa.Column("relation_status", sa.String(length=16), nullable=False),
        sa.Column("visibility", sa.String(length=16), nullable=False),
        sa.Column("first_seen_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("last_seen_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("kind IN ('post', 'comment', 'reply')", name="ck_contents_kind"),
        sa.CheckConstraint(
            "relation_status IN ('root', 'unresolved', 'resolved')",
            name="ck_contents_relation_status",
        ),
        sa.CheckConstraint(
            "visibility IN ('available', 'unavailable', 'deleted')",
            name="ck_contents_visibility",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "source",
            "provider_namespace",
            "external_id",
            name="uq_contents_provider_identity",
        ),
    )
    op.create_index(op.f("ix_contents_source"), "contents", ["source"], unique=False)
    op.create_index(op.f("ix_contents_last_seen_at"), "contents", ["last_seen_at"], unique=False)
    op.create_index("ix_contents_inbox_order", "contents", ["first_seen_at", "id"], unique=False)
    op.create_table(
        "content_versions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("content_id", sa.Uuid(), nullable=False),
        sa.Column("version", sa.Integer(), nullable=False),
        sa.Column("text", sa.Text(), nullable=False),
        sa.Column("text_sha256", sa.String(length=64), nullable=False),
        sa.Column("published_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("observed_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("raw_page_id", sa.Uuid(), nullable=False),
        sa.CheckConstraint("version > 0", name="ck_content_versions_version"),
        sa.ForeignKeyConstraint(["content_id"], ["contents.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["raw_page_id"], ["raw_pages.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("content_id", "version", name="uq_content_versions_number"),
        sa.UniqueConstraint("content_id", "text_sha256", name="uq_content_versions_text"),
    )
    op.create_index(
        op.f("ix_content_versions_content_id"),
        "content_versions",
        ["content_id"],
        unique=False,
    )
    op.create_table(
        "content_observations",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("content_id", sa.Uuid(), nullable=False),
        sa.Column("observed_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("reply_count", sa.Integer(), nullable=True),
        sa.Column("raw_page_id", sa.Uuid(), nullable=False),
        sa.CheckConstraint(
            "reply_count IS NULL OR reply_count >= 0",
            name="ck_content_observations_reply_count",
        ),
        sa.ForeignKeyConstraint(["content_id"], ["contents.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["raw_page_id"], ["raw_pages.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("content_id", "raw_page_id", name="uq_content_observations_page"),
    )
    op.create_index(
        op.f("ix_content_observations_content_id"),
        "content_observations",
        ["content_id"],
        unique=False,
    )
    op.create_index(
        op.f("ix_content_observations_observed_at"),
        "content_observations",
        ["observed_at"],
        unique=False,
    )
    op.create_table(
        "monitor_matches",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("monitor_version_id", sa.Uuid(), nullable=False),
        sa.Column("content_id", sa.Uuid(), nullable=False),
        sa.Column("match_reason", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("relevance_status", sa.String(length=20), nullable=False),
        sa.Column("review_state", sa.String(length=16), nullable=False),
        sa.Column("first_seen_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("last_seen_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "relevance_status IN ('pending', 'accepted', 'rejected', 'needs_review')",
            name="ck_monitor_matches_relevance",
        ),
        sa.CheckConstraint(
            "review_state IN ('new', 'ignored', 'following')",
            name="ck_monitor_matches_review_state",
        ),
        sa.ForeignKeyConstraint(["content_id"], ["contents.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(
            ["monitor_version_id"], ["monitor_versions.id"], ondelete="CASCADE"
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "monitor_version_id",
            "content_id",
            name="uq_monitor_matches_version_content",
        ),
    )
    op.create_index(
        op.f("ix_monitor_matches_content_id"),
        "monitor_matches",
        ["content_id"],
        unique=False,
    )
    op.create_index(
        op.f("ix_monitor_matches_monitor_version_id"),
        "monitor_matches",
        ["monitor_version_id"],
        unique=False,
    )
    op.create_table(
        "collection_checkpoints",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("run_id", sa.Uuid(), nullable=False),
        sa.Column("page_key", sa.String(length=128), nullable=False),
        sa.Column("cursor", sa.String(length=2048), nullable=True),
        sa.Column("raw_page_id", sa.Uuid(), nullable=False),
        sa.Column("item_count", sa.Integer(), nullable=False),
        sa.Column("committed_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("item_count >= 0", name="ck_collection_checkpoints_item_count"),
        sa.ForeignKeyConstraint(["raw_page_id"], ["raw_pages.id"]),
        sa.ForeignKeyConstraint(["run_id"], ["collection_runs.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("run_id", "page_key", name="uq_collection_checkpoints_page"),
    )
    op.create_index(
        op.f("ix_collection_checkpoints_run_id"),
        "collection_checkpoints",
        ["run_id"],
        unique=False,
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
