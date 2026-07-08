package dto

import "time"

// Report represents a generated report.
type Report struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"-"`
	ReportType   string     `json:"report_type"`
	PeriodStart  time.Time  `json:"period_start"`
	PeriodEnd    time.Time  `json:"period_end"`
	Subject      string     `json:"subject"`
	Summary      string     `json:"summary"`
	Content      string     `json:"content"`
	HotspotCount int        `json:"hotspot_count"`
	Status       string     `json:"status"`
	SentAt       *time.Time `json:"sent_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateInput holds data for creating a report.
type CreateInput struct {
	ReportType  string
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	Send        bool
	MonitorID   int64
}

// ListFilter defines filtering and pagination for report List queries.
type ListFilter struct {
	UserID     int64
	ReportType string
	Limit      int
	Offset     int
}

// CreateReportRecord contains fields for persisting a new report.
type CreateReportRecord struct {
	UserID       int64
	ReportType   string
	PeriodStart  time.Time
	PeriodEnd    time.Time
	Subject      string
	Summary      string
	Content      string
	HotspotCount int
	Status       string
}

// MonitorSource is a lightweight monitor reference for report generation.
type MonitorSource struct {
	ID     int64
	UserID int64
	Name   string
}

// TopicSource is a lightweight topic reference for report generation.
type TopicSource struct {
	ID        int64
	MonitorID int64
	Title     string
	Summary   string
	HeatScore float64
	Trend     string
	PostCount int
}

// PostSource is a lightweight post reference for report generation.
type PostSource struct {
	ID        int64
	MonitorID int64
	Title     string
	Content   string
	URL       string
	Platform  string
	HeatScore float64
}
