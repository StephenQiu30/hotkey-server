package db

import _ "embed"

// SchemaSQL is the canonical PostgreSQL DDL (13 tables).
//
//go:embed schema.sql
var SchemaSQL string
