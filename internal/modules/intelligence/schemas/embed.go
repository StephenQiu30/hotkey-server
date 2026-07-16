// Package schemas exposes immutable, versioned AI schema resources.
package schemas

import "embed"

// Files is intentionally the single source for static AI schema and
// instruction assets. Callers receive copies from the embedded filesystem.
//
//go:embed v1/*
var Files embed.FS
