package report

import (
	"errors"
	"time"
)

const (
	TypeDaily  = "daily"
	TypeWeekly = "weekly"

	StatusDraft = "draft"
	StatusSent  = "sent"
)

var (
	ErrInvalidInput    = errors.New("invalid report input")
	ErrNoReportSources = errors.New("no report sources")
	ErrNotFound        = errors.New("report not found")
	ErrUnsupportedType = errors.New("unsupported report type")
)

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

type CreateInput struct {
	ReportType  string
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	Send        bool
	MonitorID   int64
}

type ListFilter struct {
	UserID     int64
	ReportType string
	Limit      int
	Offset     int
}

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

type MonitorSource struct {
	ID     int64
	UserID int64
	Name   string
}

type TopicSource struct {
	ID        int64
	MonitorID int64
	Title     string
	Summary   string
	HeatScore float64
	Trend     string
	PostCount int
}

type PostSource struct {
	ID        int64
	MonitorID int64
	Title     string
	Content   string
	URL       string
	Platform  string
	HeatScore float64
}
