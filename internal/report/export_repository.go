package report

import (
	"context"
	"time"
)

const (
	ExportKindDailyDigest  = "daily-digest"
	ExportKindPublishDraft = "publish-draft"

	ExportStatusPending   = "pending"
	ExportStatusPublished = "published"
	ExportStatusSkipped   = "skipped"
	ExportStatusFailed    = "failed"
)

type ReportExport struct {
	ID           int64      `json:"id"`
	ReportID     int64      `json:"report_id"`
	ExportKind   string     `json:"export_kind"`
	TargetPath   string     `json:"target_path"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"error_message"`
	PublishedAt  *time.Time `json:"published_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type CreateReportExportInput struct {
	ReportID   int64
	ExportKind string
	TargetPath string
}

type ExportRepository interface {
	CreatePending(ctx context.Context, input CreateReportExportInput) (ReportExport, error)
	MarkPublished(ctx context.Context, reportID int64, exportKind string, path string, publishedAt time.Time) (ReportExport, error)
	MarkSkipped(ctx context.Context, reportID int64, exportKind string, path string, skippedAt time.Time) (ReportExport, error)
	MarkFailed(ctx context.Context, reportID int64, exportKind string, path string, message string, failedAt time.Time) (ReportExport, error)
	ListByReport(ctx context.Context, reportID int64) ([]ReportExport, error)
}
