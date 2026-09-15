"""Add collection schedule slots and daily request reservations.

Revision ID: 0007_collection_budget
Revises: 0006_collection_jobs
Create Date: 2026-09-15 22:30:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0007_collection_budget"
down_revision: str | Sequence[str] | None = "0006_collection_jobs"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.execute(
        """
        UPDATE monitor_versions
        SET schedule = schedule || '{"retention_days": 7}'::jsonb
        WHERE NOT schedule ? 'retention_days'
        """
    )
    op.add_column("collection_runs", sa.Column("trigger", sa.String(16), nullable=True))
    op.add_column(
        "collection_runs",
        sa.Column("schedule_slot", sa.DateTime(timezone=True), nullable=True),
    )
    op.add_column("collection_runs", sa.Column("budget_day", sa.Date(), nullable=True))
    op.add_column("collection_runs", sa.Column("reserved_requests", sa.Integer(), nullable=True))
    op.create_table(
        "collection_budget_usage",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("monitor_version_id", sa.Uuid(), nullable=False),
        sa.Column("budget_day", sa.Date(), nullable=False),
        sa.Column("limit_requests", sa.Integer(), nullable=False),
        sa.Column("reserved_requests", sa.Integer(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("limit_requests > 0", name="ck_collection_budget_limit"),
        sa.CheckConstraint(
            "reserved_requests BETWEEN 0 AND limit_requests",
            name="ck_collection_budget_reserved",
        ),
        sa.ForeignKeyConstraint(
            ["monitor_version_id"], ["monitor_versions.id"], ondelete="CASCADE"
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "monitor_version_id", "budget_day", name="uq_collection_budget_usage_day"
        ),
    )
    op.create_index(
        op.f("ix_collection_budget_usage_monitor_version_id"),
        "collection_budget_usage",
        ["monitor_version_id"],
        unique=False,
    )
    op.execute(
        """
        UPDATE collection_runs
        SET
            trigger = 'manual',
            budget_day = (created_at AT TIME ZONE 'UTC')::date,
            reserved_requests = 1
        """
    )
    op.execute(
        """
        INSERT INTO collection_budget_usage (
            id, monitor_version_id, budget_day, limit_requests,
            reserved_requests, created_at, updated_at
        )
        SELECT
            md5(
                runs.monitor_version_id::text || '-' || runs.budget_day::text ||
                '-collection-budget'
            )::uuid,
            runs.monitor_version_id,
            runs.budget_day,
            GREATEST((versions.budget->>'daily_requests')::integer, COUNT(*)::integer),
            COUNT(*)::integer,
            MIN(runs.created_at),
            MAX(runs.created_at)
        FROM collection_runs AS runs
        JOIN monitor_versions AS versions ON versions.id = runs.monitor_version_id
        GROUP BY runs.monitor_version_id, runs.budget_day, versions.budget
        """
    )
    op.alter_column("collection_runs", "trigger", nullable=False)
    op.alter_column("collection_runs", "budget_day", nullable=False)
    op.alter_column("collection_runs", "reserved_requests", nullable=False)
    op.create_check_constraint(
        "ck_collection_runs_trigger",
        "collection_runs",
        "trigger IN ('manual', 'scheduled')",
    )
    op.create_check_constraint(
        "ck_collection_runs_schedule_slot",
        "collection_runs",
        "(trigger = 'manual' AND schedule_slot IS NULL) OR "
        "(trigger = 'scheduled' AND schedule_slot IS NOT NULL)",
    )
    op.create_check_constraint(
        "ck_collection_runs_reserved_requests",
        "collection_runs",
        "reserved_requests = 1",
    )
    op.create_unique_constraint(
        "uq_collection_runs_schedule_slot",
        "collection_runs",
        [
            "monitor_version_id",
            "source",
            "operation",
            "query_variant",
            "schedule_slot",
        ],
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
