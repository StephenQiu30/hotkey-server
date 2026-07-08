package obsidian

import "errors"

var (
	ErrMissingVaultRoot  = errors.New("missing obsidian vault root")
	ErrInvalidExportKind = errors.New("invalid obsidian export kind")
)
