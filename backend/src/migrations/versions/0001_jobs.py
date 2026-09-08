"""Independent Python task ledger; never stamps an existing Go database."""

import sqlalchemy as sa
from alembic import op

revision = "0001_jobs"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "jobs",
        sa.Column("id", sa.Uuid(), primary_key=True),
        sa.Column("key", sa.String(128), nullable=False, unique=True),
        sa.Column("kind", sa.String(40), nullable=False),
        sa.Column("status", sa.String(16), nullable=False),
        sa.Column("epoch", sa.Integer(), nullable=False),
        sa.Column("attempts", sa.Integer(), nullable=False),
        sa.Column("max_attempts", sa.Integer(), nullable=False),
        sa.Column("fencing_token", sa.Integer(), nullable=False),
        sa.Column("available_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("deadline", sa.DateTime(timezone=True), nullable=False),
        sa.Column("lease_until", sa.DateTime(timezone=True)),
        sa.Column("completed_at", sa.DateTime(timezone=True)),
        sa.CheckConstraint("status IN ('queued','running','succeeded','failed','cancelled')"),
        sa.CheckConstraint("kind = 'verify_pipeline'"),
        sa.CheckConstraint("epoch > 0 AND attempts >= 0 AND max_attempts > 0"),
    )
    op.create_index("ix_jobs_recovery", "jobs", ["status", "available_at"])
    op.create_table(
        "outbox",
        sa.Column("id", sa.Uuid(), primary_key=True),
        sa.Column("job_id", sa.Uuid(), sa.ForeignKey("jobs.id"), nullable=False),
        sa.Column("epoch", sa.Integer(), nullable=False),
        sa.Column("due_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("sent_at", sa.DateTime(timezone=True)),
        sa.UniqueConstraint("job_id", "epoch"),
    )
    op.create_index("ix_outbox_due", "outbox", ["sent_at", "due_at"])
    op.create_table(
        "job_attempts",
        sa.Column("id", sa.Uuid(), primary_key=True),
        sa.Column("job_id", sa.Uuid(), sa.ForeignKey("jobs.id"), nullable=False),
        sa.Column("fencing_token", sa.Integer(), nullable=False),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("ended_at", sa.DateTime(timezone=True)),
        sa.Column("outcome", sa.String(20), nullable=False),
        sa.UniqueConstraint("job_id", "fencing_token"),
    )
    op.create_table(
        "job_results",
        sa.Column("job_id", sa.Uuid(), sa.ForeignKey("jobs.id"), primary_key=True),
        sa.Column("completed_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("result", sa.String(80), nullable=False),
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
