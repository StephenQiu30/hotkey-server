-- HotKey PostgreSQL schema
--
-- This is the only database DDL source of truth. Apply it only to a new, empty
-- database. Business tables, constraints, indexes, functions, and comments are
-- added here together with their SQLAlchemy runtime mappings.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- No business tables exist in the current backend slice.

COMMIT;
