"""Allow a post detail run to schedule one root comment page.

Revision ID: 0009_root_comments
Revises: 0008_reference_expansion
Create Date: 2026-09-15 23:55:00.000000
"""

from collections.abc import Sequence

from alembic import op

revision: str = "0009_root_comments"
down_revision: str | Sequence[str] | None = "0008_reference_expansion"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.drop_constraint("ck_collection_runs_operation", "collection_runs", type_="check")
    op.drop_constraint("ck_collection_runs_parent_operation", "collection_runs", type_="check")
    op.create_check_constraint(
        "ck_collection_runs_operation",
        "collection_runs",
        "operation IN ('search_posts', 'fetch_post', 'list_comments')",
    )
    op.create_check_constraint(
        "ck_collection_runs_parent_operation",
        "collection_runs",
        "(operation = 'search_posts' AND parent_run_id IS NULL) OR "
        "(operation IN ('fetch_post', 'list_comments') AND parent_run_id IS NOT NULL)",
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
