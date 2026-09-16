"""Account for every collection retry in the request budget.

Revision ID: 0019_collection_retry_budget
Revises: 0018_withdrawal_lifecycle
Create Date: 2026-09-16 09:30:00.000000
"""

from collections.abc import Sequence

from alembic import op

revision: str = "0019_collection_retry_budget"
down_revision: str | Sequence[str] | None = "0018_withdrawal_lifecycle"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.drop_constraint(
        "ck_collection_runs_reserved_requests",
        "collection_runs",
        type_="check",
    )
    op.create_check_constraint(
        "ck_collection_runs_reserved_requests",
        "collection_runs",
        "reserved_requests >= 1",
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
