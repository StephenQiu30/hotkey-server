// Package digest defines types and repository interfaces for daily digest exports.
package digest

import "time"

// ExportStatus represents the lifecycle of a topic daily export.
type ExportStatus string

const (
	StatusPending   ExportStatus = "pending"
	StatusPublished ExportStatus = "published"
	StatusFailed    ExportStatus = "failed"
)

// TopicDailyExport is the persisted record of a single topic's daily digest export.
type TopicDailyExport struct {
	ID           int64        `json:"id"`
	MonitorID    int64        `json:"monitor_id"`
	TopicID      int64        `json:"topic_id"`
	ExportDate   time.Time    `json:"export_date"`
	SummaryText  string       `json:"summary_text"`
	MarkdownPath string       `json:"markdown_path"`
	Status       ExportStatus `json:"status"`
	ErrorMessage string       `json:"error_message"`
	PublishedAt  *time.Time   `json:"published_at"`
	CreatedAt    time.Time    `json:"created_at"`
}

// Repository abstracts persistence for topic_daily_exports.
type Repository interface {
	// Upsert inserts or updates a daily export record for the given (monitor_id, topic_id, export_date).
	Upsert(e TopicDailyExport) (TopicDailyExport, error)

	// GetByTopicDate retrieves a daily export by topic_id and export_date.
	GetByTopicDate(topicID int64, exportDate time.Time) (*TopicDailyExport, error)
}
