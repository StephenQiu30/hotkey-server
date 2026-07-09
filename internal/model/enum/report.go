package enum

// ReportType defines the type of a report.
type ReportType string

const (
	ReportTypeDaily  ReportType = "daily"
	ReportTypeWeekly ReportType = "weekly"
)

// ReportStatus defines the lifecycle state of a report.
type ReportStatus string

const (
	ReportStatusDraft ReportStatus = "draft"
	ReportStatusSent  ReportStatus = "sent"
)

// ExportStatus defines the status of a report export operation.
type ExportStatus string

const (
	ExportStatusPending   ExportStatus = "pending"
	ExportStatusPublished ExportStatus = "published"
	ExportStatusSkipped   ExportStatus = "skipped"
	ExportStatusFailed    ExportStatus = "failed"
)

// ExportKind defines the export type (daily-digest or publish-draft).
type ExportKind string

const (
	ExportKindDailyDigest  ExportKind = "daily-digest"
	ExportKindPublishDraft ExportKind = "publish-draft"
)
