"""Bind collection runs to the durable job ledger.

Revision ID: 0006_collection_jobs
Revises: 0005_collection_page
Create Date: 2026-09-15 21:30:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "0006_collection_jobs"
down_revision: str | Sequence[str] | None = "0005_collection_page"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.drop_constraint("jobs_kind_check", "jobs", type_="check")
    op.create_check_constraint(
        "ck_jobs_kind",
        "jobs",
        "kind IN ('verify_pipeline','collect_page')",
    )
    op.add_column("collection_runs", sa.Column("job_id", sa.Uuid(), nullable=True))
    op.add_column("collection_runs", sa.Column("retention_days", sa.Integer(), nullable=True))
    op.execute(
        """
        INSERT INTO jobs (
            id, key, kind, status, epoch, attempts, max_attempts, fencing_token,
            available_at, deadline, lease_until, completed_at
        )
        SELECT
            md5(id::text || '-collection-job')::uuid,
            'collection-backfill:' || id::text,
            'collect_page',
            CASE
                WHEN state = 'completed' THEN 'succeeded'
                WHEN state = 'cancelled' THEN 'cancelled'
                ELSE 'failed'
            END,
            1, 0, 1, fencing_token, created_at, created_at + interval '1 hour', NULL,
            COALESCE(completed_at, now())
        FROM collection_runs
        """
    )
    op.execute(
        """
        UPDATE collection_runs
        SET
            job_id = md5(id::text || '-collection-job')::uuid,
            retention_days = 7,
            state = CASE WHEN state IN ('queued', 'running') THEN 'failed' ELSE state END,
            outcome = CASE WHEN state IN ('queued', 'running') THEN 'failed' ELSE outcome END,
            stop_reason = CASE
                WHEN state IN ('queued', 'running') THEN 'migration_boundary'
                ELSE stop_reason
            END,
            completed_at = CASE
                WHEN state IN ('queued', 'running') THEN now()
                ELSE completed_at
            END
        """
    )
    op.alter_column("collection_runs", "job_id", nullable=False)
    op.alter_column("collection_runs", "retention_days", nullable=False)
    op.create_foreign_key(
        "fk_collection_runs_job_id_jobs",
        "collection_runs",
        "jobs",
        ["job_id"],
        ["id"],
    )
    op.create_unique_constraint("uq_collection_runs_job_id", "collection_runs", ["job_id"])
    op.create_check_constraint(
        "ck_collection_runs_retention_days",
        "collection_runs",
        "retention_days BETWEEN 1 AND 365",
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
