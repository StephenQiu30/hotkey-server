package db

import (
	"embed"
	"io/fs"
)

// SchemaSQL is the canonical PostgreSQL DDL.
//
//go:embed schema.sql
var SchemaSQL string

// MigrationFS contains ordered SQL migrations for existing databases.
//
//go:embed migrations/*.sql
var MigrationFS embed.FS

// MigrationFiles returns embedded migration files in lexical order.
func MigrationFiles() ([]fs.DirEntry, error) {
	return fs.ReadDir(MigrationFS, "migrations")
}
