"""Create frozen analysis samples and manual labels.

Revision ID: 0015_analysis_sample_baseline
Revises: 0014_collection_ingestion_mode
Create Date: 2026-09-16 08:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0015_analysis_sample_baseline"
down_revision: str | Sequence[str] | None = "0014_collection_ingestion_mode"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "analysis_runs",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("event_id", sa.Uuid(), nullable=False),
        sa.Column("event_revision", sa.Integer(), nullable=False),
        sa.Column("state", sa.String(16), nullable=False),
        sa.Column("method", sa.String(32), nullable=False),
        sa.Column("analyzer_id", sa.String(64), nullable=False),
        sa.Column("prompt_version", sa.String(64), nullable=False),
        sa.Column("label_schema_version", sa.String(64), nullable=False),
        sa.Column("sampling_policy_version", sa.String(64), nullable=False),
        sa.Column("request_sha256", sa.String(64), nullable=False),
        sa.Column("manifest_sha256", sa.String(64), nullable=False),
        sa.Column("since_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("until_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("cutoff_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("max_items", sa.Integer(), nullable=False),
        sa.Column("sample_count", sa.Integer(), nullable=False),
        sa.Column("token_budget", sa.Integer(), nullable=False),
        sa.Column("input_tokens", sa.Integer(), nullable=False),
        sa.Column("output_tokens", sa.Integer(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("event_revision > 0", name="ck_analysis_runs_event_revision"),
        sa.CheckConstraint("since_at < until_at", name="ck_analysis_runs_window"),
        sa.CheckConstraint("until_at <= cutoff_at", name="ck_analysis_runs_cutoff"),
        sa.CheckConstraint("max_items BETWEEN 1 AND 100", name="ck_analysis_runs_max_items"),
        sa.CheckConstraint(
            "sample_count BETWEEN 1 AND max_items", name="ck_analysis_runs_sample_count"
        ),
        sa.CheckConstraint("state IN ('pending', 'succeeded')", name="ck_analysis_runs_state"),
        sa.CheckConstraint(
            "token_budget = 0 AND input_tokens = 0 AND output_tokens = 0",
            name="ck_analysis_runs_manual_tokens",
        ),
        sa.ForeignKeyConstraint(["event_id"], ["events.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("event_id", "request_sha256", name="uq_analysis_runs_request"),
    )
    op.create_index("ix_analysis_runs_event_created", "analysis_runs", ["event_id", "created_at"])
    op.create_table(
        "analysis_samples",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("run_id", sa.Uuid(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("content_id", sa.Uuid(), nullable=False),
        sa.Column("content_version_id", sa.Uuid(), nullable=False),
        sa.Column("parent_content_version_id", sa.Uuid(), nullable=True),
        sa.Column("root_content_version_id", sa.Uuid(), nullable=True),
        sa.Column("source", sa.String(32), nullable=False),
        sa.Column("kind", sa.String(16), nullable=False),
        sa.Column("root_external_id", sa.String(1024), nullable=False),
        sa.Column("published_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("time_bucket_start", sa.DateTime(timezone=True), nullable=False),
        sa.Column("ordering_origin", sa.String(32), nullable=False),
        sa.Column("selection_reason", sa.String(64), nullable=False),
        sa.Column("text_sha256", sa.String(64), nullable=False),
        sa.Column("parent_text_sha256", sa.String(64), nullable=True),
        sa.Column("root_text_sha256", sa.String(64), nullable=True),
        sa.CheckConstraint("position > 0", name="ck_analysis_samples_position"),
        sa.CheckConstraint("kind IN ('comment', 'reply')", name="ck_analysis_samples_kind"),
        sa.CheckConstraint(
            "ordering_origin IN ('provider_default')",
            name="ck_analysis_samples_ordering_origin",
        ),
        sa.ForeignKeyConstraint(["run_id"], ["analysis_runs.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["content_id"], ["contents.id"], ondelete="RESTRICT"),
        sa.ForeignKeyConstraint(
            ["content_version_id"], ["content_versions.id"], ondelete="RESTRICT"
        ),
        sa.ForeignKeyConstraint(
            ["parent_content_version_id"], ["content_versions.id"], ondelete="RESTRICT"
        ),
        sa.ForeignKeyConstraint(
            ["root_content_version_id"], ["content_versions.id"], ondelete="RESTRICT"
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("run_id", "position", name="uq_analysis_samples_position"),
        sa.UniqueConstraint("run_id", "content_version_id", name="uq_analysis_samples_version"),
    )
    op.create_index("ix_analysis_samples_run_id", "analysis_samples", ["run_id"])
    op.create_table(
        "analysis_labels",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("sample_id", sa.Uuid(), nullable=False),
        sa.Column("topic", sa.String(80), nullable=False),
        sa.Column("target", sa.String(160), nullable=False),
        sa.Column("sentiment", sa.String(16), nullable=False),
        sa.Column("stance", sa.String(16), nullable=False),
        sa.Column("request", sa.Text(), nullable=False),
        sa.Column("abstained", sa.Boolean(), nullable=False),
        sa.Column(
            "citation_content_version_ids",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
        ),
        sa.Column("label_source", sa.String(16), nullable=False),
        sa.Column("schema_version", sa.String(64), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "sentiment IN ('positive', 'negative', 'neutral', 'mixed', 'unknown')",
            name="ck_analysis_labels_sentiment",
        ),
        sa.CheckConstraint(
            "stance IN ('support', 'oppose', 'neutral', 'mixed', 'unknown')",
            name="ck_analysis_labels_stance",
        ),
        sa.CheckConstraint("label_source IN ('manual')", name="ck_analysis_labels_source"),
        sa.ForeignKeyConstraint(["sample_id"], ["analysis_samples.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("sample_id", name="uq_analysis_labels_sample"),
    )
    op.create_index("ix_analysis_labels_sample_id", "analysis_labels", ["sample_id"])


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
