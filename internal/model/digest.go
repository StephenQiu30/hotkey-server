package model

import "time"

// TopicDailyExport is a daily digest export record.
type TopicDailyExport struct {
	ID           int64
	MonitorID    int64
	TopicID      int64
	ExportDate   string
	SummaryText  string
	MarkdownPath string
	Status       string
	ErrorMessage string
	PublishedAt  *time.Time
	CreatedAt    time.Time
}
