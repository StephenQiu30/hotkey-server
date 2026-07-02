// Package export orchestrates knowledge object exports into periodic reports,
// thematic reports, and material lists that are written into the Obsidian vault.
package export

import "time"

// ExportKind identifies the type of export bundle.
type ExportKind string

const (
	ExportDaily    ExportKind = "daily"
	ExportWeekly   ExportKind = "weekly"
	ExportMonthly  ExportKind = "monthly"
	ExportThematic ExportKind = "thematic"
	ExportMaterial ExportKind = "material"
)

// DateRange represents a time range for periodic exports.
type DateRange struct {
	Start string // YYYY-MM-DD or ISO week
	End   string
}

// ExportBundle is the intermediate orchestration object that collects all data
// needed to render a single export output.
type ExportBundle struct {
	Kind       ExportKind
	DateRange  DateRange
	MonitorID  int64
	TopicIDs   []int64
	EventIDs   []int64
	ThemeIDs   []int64
	GeneratedAt time.Time
	Content    string
}

// BuildExportBundleInput is the input for building an export bundle.
type BuildExportBundleInput struct {
	Kind      string
	DateRange DateRange
	MonitorID int64
	TopicIDs  []int64
	EventIDs  []int64
	ThemeIDs  []int64
}

// BuildExportBundle creates an ExportBundle from the given input parameters.
// In a full implementation this would query the database for the actual
// data; for now it creates the bundle skeleton with the IDs and metadata.
func BuildExportBundle(in BuildExportBundleInput) ExportBundle {
	return ExportBundle{
		Kind:        ExportKind(in.Kind),
		DateRange:   in.DateRange,
		MonitorID:   in.MonitorID,
		TopicIDs:    in.TopicIDs,
		EventIDs:    in.EventIDs,
		ThemeIDs:    in.ThemeIDs,
		GeneratedAt: time.Now(),
	}
}
