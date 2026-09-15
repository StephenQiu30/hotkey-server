"""Create versioned knowledge snapshots from completed analyses.

Revision ID: 0016_analysis_knowledge
Revises: 0015_analysis_sample_baseline
Create Date: 2026-09-16 12:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0016_analysis_knowledge"
down_revision: str | Sequence[str] | None = "0015_analysis_sample_baseline"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "knowledge_entries",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("event_id", sa.Uuid(), nullable=False),
        sa.Column("source_analysis_run_id", sa.Uuid(), nullable=False),
        sa.Column("entry_type", sa.String(32), nullable=False),
        sa.Column("current_version", sa.Integer(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("entry_type IN ('analysis_snapshot')", name="ck_knowledge_entries_type"),
        sa.CheckConstraint("current_version > 0", name="ck_knowledge_entries_current_version"),
        sa.ForeignKeyConstraint(["event_id"], ["events.id"], ondelete="RESTRICT"),
        sa.ForeignKeyConstraint(
            ["source_analysis_run_id"], ["analysis_runs.id"], ondelete="RESTRICT"
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("source_analysis_run_id", name="uq_knowledge_entries_analysis_run"),
    )
    op.create_index(
        "ix_knowledge_entries_event_created",
        "knowledge_entries",
        ["event_id", "created_at"],
    )
    op.create_table(
        "knowledge_versions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("entry_id", sa.Uuid(), nullable=False),
        sa.Column("version", sa.Integer(), nullable=False),
        sa.Column("input_sha256", sa.String(64), nullable=False),
        sa.Column("analysis_manifest_sha256", sa.String(64), nullable=False),
        sa.Column("title", sa.String(240), nullable=False),
        sa.Column("body", sa.Text(), nullable=False),
        sa.Column("search_text", sa.Text(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("version > 0", name="ck_knowledge_versions_version"),
        sa.ForeignKeyConstraint(["entry_id"], ["knowledge_entries.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("entry_id", "input_sha256", name="uq_knowledge_versions_entry_input"),
        sa.UniqueConstraint("entry_id", "version", name="uq_knowledge_versions_entry_version"),
    )
    op.create_index("ix_knowledge_versions_entry_id", "knowledge_versions", ["entry_id"])
    op.create_table(
        "knowledge_citations",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("version_id", sa.Uuid(), nullable=False),
        sa.Column("content_version_id", sa.Uuid(), nullable=False),
        sa.Column("text_sha256", sa.String(64), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.CheckConstraint("position > 0", name="ck_knowledge_citations_position"),
        sa.ForeignKeyConstraint(
            ["content_version_id"], ["content_versions.id"], ondelete="RESTRICT"
        ),
        sa.ForeignKeyConstraint(["version_id"], ["knowledge_versions.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "version_id",
            "content_version_id",
            name="uq_knowledge_citations_content_version",
        ),
        sa.UniqueConstraint("version_id", "position", name="uq_knowledge_citations_position"),
    )
    op.create_index("ix_knowledge_citations_version_id", "knowledge_citations", ["version_id"])


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
