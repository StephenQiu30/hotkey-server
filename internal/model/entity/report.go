package entity

import "time"

type Report struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	UserID       int64      `gorm:"column:user_id"`
	ReportType   string     `gorm:"column:report_type"`
	PeriodStart  time.Time  `gorm:"column:period_start;type:date"`
	PeriodEnd    time.Time  `gorm:"column:period_end;type:date"`
	Subject      string     `gorm:"column:subject"`
	Summary      string     `gorm:"column:summary"`
	Content      string     `gorm:"column:content"`
	HotspotCount int        `gorm:"column:hotspot_count"`
	Status       string     `gorm:"column:status"`
	SentAt       *time.Time `gorm:"column:sent_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (Report) TableName() string { return "reports" }

type ReportExport struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	ReportID     int64      `gorm:"column:report_id"`
	ExportKind   string     `gorm:"column:export_kind"`
	TargetPath   string     `gorm:"column:target_path"`
	Status       string     `gorm:"column:status"`
	ErrorMessage string     `gorm:"column:error_message"`
	PublishedAt  *time.Time `gorm:"column:published_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (ReportExport) TableName() string { return "report_exports" }
