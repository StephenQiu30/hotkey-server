"""Persist content withdrawal tombstones and evidence cleanup state.

Revision ID: 0018_withdrawal_lifecycle
Revises: 0017_knowledge_semantic_index
Create Date: 2026-09-16 08:15:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0018_withdrawal_lifecycle"
down_revision: str | Sequence[str] | None = "0017_knowledge_semantic_index"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "raw_pages",
        sa.Column("object_state", sa.String(length=16), server_default="available", nullable=False),
    )
    op.add_column(
        "raw_pages",
        sa.Column("cleanup_attempts", sa.Integer(), server_default="0", nullable=False),
    )
    op.add_column("raw_pages", sa.Column("cleanup_error_code", sa.String(length=64), nullable=True))
    op.add_column("raw_pages", sa.Column("deleted_at", sa.DateTime(timezone=True), nullable=True))
    op.create_check_constraint(
        "ck_raw_pages_object_state",
        "raw_pages",
        "object_state IN ('available', 'delete_pending', 'deleted', 'failed')",
    )
    op.create_check_constraint(
        "ck_raw_pages_cleanup_attempts", "raw_pages", "cleanup_attempts >= 0"
    )
    op.create_index(
        "ix_raw_pages_object_cleanup",
        "raw_pages",
        ["object_state", "observed_at", "id"],
        unique=False,
    )
    op.create_table(
        "content_withdrawal_records",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("source", sa.String(length=32), nullable=False),
        sa.Column("provider_namespace", sa.String(length=100), nullable=False),
        sa.Column("external_id", sa.String(length=1024), nullable=False),
        sa.Column("visibility", sa.String(length=16), nullable=False),
        sa.Column("requested_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "visibility IN ('unavailable', 'deleted')",
            name="ck_content_withdrawal_records_visibility",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "source",
            "provider_namespace",
            "external_id",
            name="uq_content_withdrawal_records_identity",
        ),
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
