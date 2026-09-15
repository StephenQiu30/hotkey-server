"""Add persisted event change notifications.

Revision ID: 0013_event_notifications
Revises: 0012_event_merge_split
Create Date: 2026-09-16 00:30:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0013_event_notifications"
down_revision: str | Sequence[str] | None = "0012_event_merge_split"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "notifications",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("event_id", sa.Uuid(), nullable=False),
        sa.Column("change_id", sa.Uuid(), nullable=False),
        sa.Column("rule_version", sa.Integer(), nullable=False),
        sa.Column("kind", sa.String(length=32), nullable=False),
        sa.Column("message", sa.String(length=300), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("read_at", sa.DateTime(timezone=True), nullable=True),
        sa.CheckConstraint("rule_version > 0", name="ck_notifications_rule_version"),
        sa.CheckConstraint(
            "kind IN ('event_member_added', 'event_member_removed', "
            "'event_merged_in', 'event_merged_out', 'event_split_in', 'event_split_out')",
            name="ck_notifications_kind",
        ),
        sa.CheckConstraint(
            "read_at IS NULL OR read_at >= created_at", name="ck_notifications_read_time"
        ),
        sa.ForeignKeyConstraint(["change_id"], ["event_revisions.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["event_id"], ["events.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "rule_version", "event_id", "change_id", name="uq_notifications_event_change"
        ),
    )
    op.create_index(op.f("ix_notifications_change_id"), "notifications", ["change_id"])
    op.create_index(op.f("ix_notifications_created_at"), "notifications", ["created_at"])
    op.create_index(op.f("ix_notifications_event_id"), "notifications", ["event_id"])


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
