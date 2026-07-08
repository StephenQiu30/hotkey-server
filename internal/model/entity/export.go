package entity

import "time"

type TopicDailyExport struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	MonitorID    int64      `gorm:"column:monitor_id"`
	TopicID      int64      `gorm:"column:topic_id"`
	ExportDate   string     `gorm:"column:export_date"`
	SummaryText  string     `gorm:"column:summary_text"`
	MarkdownPath string     `gorm:"column:markdown_path"`
	Status       string     `gorm:"column:status"`
	ErrorMessage string     `gorm:"column:error_message"`
	PublishedAt  *time.Time `gorm:"column:published_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
}

func (TopicDailyExport) TableName() string { return "topic_daily_exports" }

type ExportBundle struct {
	ID         int64      `gorm:"column:id;primaryKey"`
	MonitorID  int64      `gorm:"column:monitor_id"`
	BundleKey  string     `gorm:"column:bundle_key"`
	BundleKind string     `gorm:"column:bundle_kind"`
	DateStart  *time.Time `gorm:"column:date_start;type:date"`
	DateEnd    *time.Time `gorm:"column:date_end;type:date"`
	Status     string     `gorm:"column:status"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
}

func (ExportBundle) TableName() string { return "export_bundles" }
