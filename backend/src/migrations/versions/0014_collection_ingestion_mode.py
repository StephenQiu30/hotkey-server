"""Distinguish live collection from historical backfill.

Revision ID: 0014_collection_ingestion_mode
Revises: 0013_event_notifications
Create Date: 2026-09-16 03:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0014_collection_ingestion_mode"
down_revision: str | Sequence[str] | None = "0013_event_notifications"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column("collection_runs", sa.Column("ingestion_mode", sa.String(16), nullable=True))
    op.execute("UPDATE collection_runs SET ingestion_mode = 'live'")
    op.alter_column("collection_runs", "ingestion_mode", nullable=False)
    op.create_check_constraint(
        "ck_collection_runs_ingestion_mode",
        "collection_runs",
        "ingestion_mode IN ('live', 'backfill')",
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
