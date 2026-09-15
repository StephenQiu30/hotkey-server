"""Record linked event merge and split revisions.

Revision ID: 0012_event_merge_split
Revises: 0011_event_dossiers
Create Date: 2026-09-16 00:20:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0012_event_merge_split"
down_revision: str | Sequence[str] | None = "0011_event_dossiers"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column("event_revisions", sa.Column("related_event_id", sa.Uuid(), nullable=True))
    op.create_foreign_key(
        "fk_event_revisions_related_event_id",
        "event_revisions",
        "events",
        ["related_event_id"],
        ["id"],
    )
    op.drop_constraint("ck_event_revisions_change_type", "event_revisions", type_="check")
    op.create_check_constraint(
        "ck_event_revisions_change_type",
        "event_revisions",
        "change_type IN ('create', 'add_member', 'remove_member', "
        "'merge_in', 'merge_out', 'split_in', 'split_out')",
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
