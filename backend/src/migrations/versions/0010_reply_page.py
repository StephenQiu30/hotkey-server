"""Allow a root comment run to schedule one reply page.

Revision ID: 0010_reply_page
Revises: 0009_root_comments
Create Date: 2026-09-15 23:30:00.000000
"""

from collections.abc import Sequence

from alembic import op

revision: str = "0010_reply_page"
down_revision: str | Sequence[str] | None = "0009_root_comments"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.drop_constraint("ck_collection_runs_operation", "collection_runs", type_="check")
    op.drop_constraint("ck_collection_runs_parent_operation", "collection_runs", type_="check")
    op.create_check_constraint(
        "ck_collection_runs_operation",
        "collection_runs",
        "operation IN ('search_posts', 'fetch_post', 'list_comments', 'list_replies')",
    )
    op.create_check_constraint(
        "ck_collection_runs_parent_operation",
        "collection_runs",
        "(operation = 'search_posts' AND parent_run_id IS NULL) OR "
        "(operation IN ('fetch_post', 'list_comments', 'list_replies') "
        "AND parent_run_id IS NOT NULL)",
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
