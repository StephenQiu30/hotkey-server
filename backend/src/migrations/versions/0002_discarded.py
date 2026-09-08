"""Separate deliberately discarded messages from broker-confirmed sends."""

import sqlalchemy as sa
from alembic import op

revision = "0002_discarded"
down_revision = "0001_jobs"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column("outbox", sa.Column("discarded_at", sa.DateTime(timezone=True)))


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported")
