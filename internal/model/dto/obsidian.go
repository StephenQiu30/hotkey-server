package dto

import "time"

// ExportKind identifies the type of Obsidian export.
type ExportKind string

const (
	ExportDailyDigest  ExportKind = "daily-digest"
	ExportPublishDraft ExportKind = "publish-draft"
)

const (
	WriteStatusPublished = "published"
	WriteStatusSkipped   = "skipped"
)

// PathInput holds parameters for building an Obsidian export path.
type PathInput struct {
	Kind        ExportKind
	Date        time.Time
	MonitorName string
}

// WriteResult holds the result of an Obsidian file write operation.
type WriteResult struct {
	Path    string
	Status  string
	Skipped bool
}
