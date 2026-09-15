"""Immutable monitor configuration versions

Revision ID: 0004_monitor_versions
Revises: 0003_workspace
Create Date: 2026-09-15 12:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0004_monitor_versions"
down_revision: str | Sequence[str] | None = "0003_workspace"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "monitors",
        sa.Column("state", sa.String(length=16), server_default="draft", nullable=False),
    )
    op.add_column("monitors", sa.Column("current_version", sa.Integer(), nullable=True))
    op.add_column("monitors", sa.Column("updated_at", sa.DateTime(timezone=True), nullable=True))
    op.create_table(
        "monitor_versions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("monitor_id", sa.Uuid(), nullable=False),
        sa.Column("version", sa.Integer(), nullable=False),
        sa.Column("title", sa.String(length=100), nullable=False),
        sa.Column("query_spec", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("source_ids", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("schedule", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("budget", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("version > 0", name="ck_monitor_versions_version"),
        sa.ForeignKeyConstraint(["monitor_id"], ["monitors.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("monitor_id", "version", name="uq_monitor_versions_monitor_version"),
    )
    op.create_index(
        op.f("ix_monitor_versions_monitor_id"),
        "monitor_versions",
        ["monitor_id"],
        unique=False,
    )
    op.execute(
        """
        INSERT INTO monitor_versions (
            id, monitor_id, version, title, query_spec, source_ids, schedule, budget, created_at
        )
        SELECT
            md5(id::text || '-monitor-version-' || version::text)::uuid,
            id,
            version,
            title,
            jsonb_build_object(
                'include_any', keywords,
                'include_all', '[]'::jsonb,
                'exclude', '[]'::jsonb,
                'aliases', '[]'::jsonb
            ),
            sources,
            jsonb_build_object('interval_minutes', 60),
            jsonb_build_object('daily_requests', 24, 'content_purchase_cost', 0),
            created_at
        FROM monitors
        """
    )
    op.execute("UPDATE monitors SET current_version = version, updated_at = created_at")
    op.alter_column("monitors", "current_version", nullable=False)
    op.alter_column("monitors", "updated_at", nullable=False)
    op.create_check_constraint(
        "ck_monitors_state",
        "monitors",
        "state IN ('draft', 'active', 'paused')",
    )
    op.create_check_constraint("ck_monitors_current_version", "monitors", "current_version > 0")
    op.execute("ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_version_check")
    op.drop_column("monitors", "keywords")
    op.drop_column("monitors", "sources")
    op.drop_column("monitors", "title")
    op.drop_column("monitors", "version")
    op.alter_column("monitors", "state", server_default=None)


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
