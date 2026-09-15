"""Add parent runs for search reference expansion.

Revision ID: 0008_reference_expansion
Revises: 0007_collection_budget
Create Date: 2026-09-15 23:30:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0008_reference_expansion"
down_revision: str | Sequence[str] | None = "0007_collection_budget"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.alter_column("collection_runs", "query_variant", new_column_name="request_value")
    op.add_column("collection_runs", sa.Column("parent_run_id", sa.Uuid(), nullable=True))
    op.create_foreign_key(
        "fk_collection_runs_parent_run_id",
        "collection_runs",
        "collection_runs",
        ["parent_run_id"],
        ["id"],
        ondelete="CASCADE",
    )
    op.create_index(
        op.f("ix_collection_runs_parent_run_id"),
        "collection_runs",
        ["parent_run_id"],
        unique=False,
    )
    op.create_check_constraint(
        "ck_collection_runs_operation",
        "collection_runs",
        "operation IN ('search_posts', 'fetch_post')",
    )
    op.create_check_constraint(
        "ck_collection_runs_parent_operation",
        "collection_runs",
        "(operation = 'search_posts' AND parent_run_id IS NULL) OR "
        "(operation = 'fetch_post' AND parent_run_id IS NOT NULL)",
    )
    op.create_check_constraint(
        "ck_collection_runs_parent_not_self",
        "collection_runs",
        "parent_run_id IS NULL OR parent_run_id <> id",
    )
    op.create_unique_constraint(
        "uq_collection_runs_parent_request",
        "collection_runs",
        ["parent_run_id", "operation", "request_value"],
    )
    op.drop_constraint("uq_collection_runs_schedule_slot", "collection_runs", type_="unique")
    op.create_index(
        "uq_collection_runs_schedule_slot",
        "collection_runs",
        ["monitor_version_id", "source", "operation", "request_value", "schedule_slot"],
        unique=True,
        postgresql_where=sa.text("parent_run_id IS NULL"),
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
