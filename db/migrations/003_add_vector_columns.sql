-- Migration 003: Add pgvector extension and embedding columns.
-- Requires: pgvector extension to be installed (CREATE EXTENSION vector).
-- Run: psql -d hotkey -f db/migrations/003_add_vector_columns.sql

CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE platform_posts ADD COLUMN IF NOT EXISTS embedding vector(384);
CREATE INDEX IF NOT EXISTS idx_platform_posts_embedding ON platform_posts
  USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

ALTER TABLE keyword_monitors ADD COLUMN IF NOT EXISTS query_embedding vector(384);
