"""Add comparable trend alert rules and occurrences.

Revision ID: 0020_trend_alerts
Revises: 0019_collection_retry_budget
Create Date: 2026-09-16 12:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0020_trend_alerts"
down_revision: str | Sequence[str] | None = "0019_collection_retry_budget"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "trend_alert_rules",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("event_id", sa.Uuid(), nullable=False),
        sa.Column("source", sa.String(length=32), nullable=False),
        sa.Column("metric", sa.String(length=32), nullable=False),
        sa.Column("bucket_hours", sa.Integer(), nullable=False),
        sa.Column("threshold_count", sa.Integer(), nullable=False),
        sa.Column("version", sa.Integer(), nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("bucket_hours IN (1, 6, 24)", name="ck_trend_alert_rules_bucket"),
        sa.CheckConstraint(
            "metric IN ('new_posts', 'new_discussions', 'observed_reply_delta')",
            name="ck_trend_alert_rules_metric",
        ),
        sa.CheckConstraint("threshold_count > 0", name="ck_trend_alert_rules_threshold"),
        sa.CheckConstraint("updated_at >= created_at", name="ck_trend_alert_rules_time"),
        sa.CheckConstraint("version > 0", name="ck_trend_alert_rules_version"),
        sa.ForeignKeyConstraint(["event_id"], ["events.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "event_id", "source", "metric", "bucket_hours", name="uq_trend_alert_rules_scope"
        ),
    )
    op.create_index(op.f("ix_trend_alert_rules_event_id"), "trend_alert_rules", ["event_id"])
    op.create_table(
        "trend_alert_occurrences",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("rule_id", sa.Uuid(), nullable=False),
        sa.Column("event_id", sa.Uuid(), nullable=False),
        sa.Column("rule_version", sa.Integer(), nullable=False),
        sa.Column("bucket_start", sa.DateTime(timezone=True), nullable=False),
        sa.Column("bucket_end", sa.DateTime(timezone=True), nullable=False),
        sa.Column("metric_value", sa.Integer(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("bucket_end > bucket_start", name="ck_trend_alert_occurrences_window"),
        sa.CheckConstraint("metric_value >= 0", name="ck_trend_alert_occurrences_value"),
        sa.CheckConstraint("rule_version > 0", name="ck_trend_alert_occurrences_version"),
        sa.ForeignKeyConstraint(["event_id"], ["events.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["rule_id"], ["trend_alert_rules.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "rule_id", "rule_version", "bucket_start", name="uq_trend_alert_occurrences_bucket"
        ),
    )
    op.create_index(
        op.f("ix_trend_alert_occurrences_created_at"),
        "trend_alert_occurrences",
        ["created_at"],
    )
    op.create_index(
        op.f("ix_trend_alert_occurrences_event_id"), "trend_alert_occurrences", ["event_id"]
    )
    op.create_index(
        op.f("ix_trend_alert_occurrences_rule_id"), "trend_alert_occurrences", ["rule_id"]
    )
    op.alter_column("notifications", "change_id", existing_type=sa.Uuid(), nullable=True)
    op.add_column("notifications", sa.Column("trend_occurrence_id", sa.Uuid(), nullable=True))
    op.drop_constraint("ck_notifications_kind", "notifications", type_="check")
    op.create_check_constraint(
        "ck_notifications_kind",
        "notifications",
        "kind IN ('event_member_added', 'event_member_removed', 'event_merged_in', "
        "'event_merged_out', 'event_split_in', 'event_split_out', 'trend_threshold_reached')",
    )
    op.create_check_constraint(
        "ck_notifications_exactly_one_subject",
        "notifications",
        "(change_id IS NULL) <> (trend_occurrence_id IS NULL)",
    )
    op.create_foreign_key(
        "fk_notifications_trend_occurrence_id_trend_alert_occurrences",
        "notifications",
        "trend_alert_occurrences",
        ["trend_occurrence_id"],
        ["id"],
        ondelete="CASCADE",
    )
    op.create_unique_constraint(
        "uq_notifications_trend_occurrence", "notifications", ["trend_occurrence_id"]
    )
    op.create_index(
        op.f("ix_notifications_trend_occurrence_id"),
        "notifications",
        ["trend_occurrence_id"],
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
