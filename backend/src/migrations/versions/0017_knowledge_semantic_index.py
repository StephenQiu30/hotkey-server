"""Add recoverable semantic indexes for knowledge versions.

Revision ID: 0017_knowledge_semantic_index
Revises: 0016_analysis_knowledge
Create Date: 2026-09-16 15:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op
from pgvector.sqlalchemy import Vector

revision: str = "0017_knowledge_semantic_index"
down_revision: str | Sequence[str] | None = "0016_analysis_knowledge"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.execute("CREATE EXTENSION IF NOT EXISTS vector")
    op.create_table(
        "knowledge_chunks",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("version_id", sa.Uuid(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("text", sa.Text(), nullable=False),
        sa.Column("text_sha256", sa.String(64), nullable=False),
        sa.Column("embedding", Vector(1024), nullable=True),
        sa.Column("embedding_model", sa.String(120), nullable=True),
        sa.Column("embedding_model_digest", sa.String(64), nullable=True),
        sa.Column("embedding_dimensions", sa.Integer(), nullable=True),
        sa.Column("index_state", sa.String(16), nullable=False),
        sa.Column("error_code", sa.String(64), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("indexed_at", sa.DateTime(timezone=True), nullable=True),
        sa.CheckConstraint("position = 1", name="ck_knowledge_chunks_position"),
        sa.CheckConstraint(
            "index_state IN ('pending', 'ready', 'failed', 'stale', 'deleted')",
            name="ck_knowledge_chunks_index_state",
        ),
        sa.CheckConstraint(
            "embedding_model_digest IS NULL OR embedding_model_digest ~ '^[0-9a-f]{64}$'",
            name="ck_knowledge_chunks_model_digest",
        ),
        sa.CheckConstraint(
            "(index_state = 'ready' AND embedding IS NOT NULL AND embedding_model IS NOT NULL "
            "AND embedding_model_digest IS NOT NULL AND embedding_dimensions = 1024 "
            "AND indexed_at IS NOT NULL AND error_code IS NULL) "
            "OR (index_state <> 'ready' AND embedding IS NULL)",
            name="ck_knowledge_chunks_ready_fields",
        ),
        sa.ForeignKeyConstraint(["version_id"], ["knowledge_versions.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("version_id", name="uq_knowledge_chunks_version"),
    )
    op.create_index(
        "ix_knowledge_chunks_state_model",
        "knowledge_chunks",
        ["index_state", "embedding_model"],
    )
    op.execute(
        """
        INSERT INTO knowledge_chunks (
            id, version_id, position, text, text_sha256, index_state, created_at
        )
        SELECT
            id, id, 1, title || E'\\n' || body,
            encode(sha256(convert_to(title || E'\\n' || body, 'UTF8')), 'hex'),
            'pending', created_at
        FROM knowledge_versions
        """
    )


def downgrade() -> None:
    raise RuntimeError("Destructive downgrade is not supported; restore a verified backup")
