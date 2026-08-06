// Package db owns the application's single executable database structure.
//
// schema.sql remains the canonical source; embedding it prevents runtime
// commands from reading a second copy from the filesystem or drifting from the
// schema reviewed with the application binary.
package db

import _ "embed"

// SchemaSQL is the canonical HotKey schema used by db init and db verify.
//
//go:embed schema.sql
var SchemaSQL string
