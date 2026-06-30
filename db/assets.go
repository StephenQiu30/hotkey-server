package db

import _ "embed"

// SchemaSQL is the canonical PostgreSQL DDL.
//
//go:embed schema.sql
var SchemaSQL string
