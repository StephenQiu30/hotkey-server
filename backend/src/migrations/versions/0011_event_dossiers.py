"""Create manual event dossiers and revision snapshots.

Revision ID: 0011_event_dossiers
Revises: 0010_reply_page
Create Date: 2026-09-15 23:55:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "0011_event_dossiers"
down_revision: str | Sequence[str] | None = "0010_reply_page"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "events",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("title", sa.String(length=200), nullable=False),
        sa.Column("summary", sa.Text(), nullable=False),
        sa.Column("status", sa.String(length=16), nullable=False),
        sa.Column("current_revision", sa.Integer(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("current_revision > 0", name="ck_events_current_revision"),
        sa.CheckConstraint("status IN ('active', 'archived')", name="ck_events_status"),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_table(
        "event_members",
        sa.Column("event_id", sa.Uuid(), nullable=False),
        sa.Column("content_id", sa.Uuid(), nullable=False),
        sa.Column("added_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["content_id"], ["contents.id"], ondelete="RESTRICT"),
        sa.ForeignKeyConstraint(["event_id"], ["events.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("event_id", "content_id"),
        sa.UniqueConstraint("content_id", name="uq_event_members_content"),
    )
    op.create_table(
        "event_revisions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("event_id", sa.Uuid(), nullable=False),
        sa.Column("revision", sa.Integer(), nullable=False),
        sa.Column("change_type", sa.String(length=32), nullable=False),
        sa.Column("snapshot", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "change_type IN ('create', 'add_member', 'remove_member')",
            name="ck_event_revisions_change_type",
        ),
        sa.CheckConstraint("revision > 0", name="ck_event_revisions_revision"),
        sa.ForeignKeyConstraint(["event_id"], ["events.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("event_id", "revision", name="uq_event_revisions_number"),
    )
    op.create_index("ix_event_revisions_event_id", "event_revisions", ["event_id"])


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
