package obsidian

import (
	"errors"
	"time"
)

type ExportKind string

const (
	ExportDailyDigest  ExportKind = "daily-digest"
	ExportPublishDraft ExportKind = "publish-draft"
)

const (
	WriteStatusPublished = "published"
	WriteStatusSkipped   = "skipped"
)

var (
	ErrMissingVaultRoot = errors.New("missing obsidian vault root")
	ErrInvalidExportKind = errors.New("invalid obsidian export kind")
)

type PathInput struct {
	Kind        ExportKind
	Date        time.Time
	MonitorName string
}

type WriteResult struct {
	Path    string
	Status  string
	Skipped bool
}
