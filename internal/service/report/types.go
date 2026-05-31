package report

import (
	"context"
	"errors"
	"time"
)

const PromptVersion = "daily_report_zh_v1"

type ReportStatus string

const (
	ReportStatusSucceeded    ReportStatus = "succeeded"
	ReportStatusDegraded     ReportStatus = "degraded"
	ReportStatusFailedConfig ReportStatus = "failed_config"
	ReportStatusFailed       ReportStatus = "failed"
)

var (
	ErrInvalidInput         = errors.New("invalid input")
	ErrNotFound             = errors.New("not found")
	ErrFailedConfig         = errors.New("failed config")
	ErrInsufficientEvidence = errors.New("insufficient evidence")
)

type SourceRef struct {
	SourceID string `json:"sourceId"`
	ItemID   string `json:"itemId"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

type DailyReport struct {
	ID              string
	Date            string
	ChannelID       string
	UserID          string
	PromptVersion   string
	InputHotspotIDs []string
	Body            string
	Status          ReportStatus
	LastError       string
	SourceRefs      []SourceRef
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AISummary struct {
	ID            string
	ClusterID     string
	PromptVersion string
	Summary       string
	Status        ReportStatus
	LastError     string
	SourceRefs    []SourceRef
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type HotspotData struct {
	Cluster ClusterInfo
	Score   ScoreInfo
	Items   []ContentItemInfo
	Sources []SourceInfo
}

type ClusterInfo struct {
	ID        string
	Title     string
	Keywords  []string
	UpdatedAt time.Time
}

type ScoreInfo struct {
	ClusterID  string
	TotalScore float64
}

type ContentItemInfo struct {
	ID          string
	SourceID    string
	Title       string
	Snippet     string
	URL         string
	PublishedAt *time.Time
	CreatedAt   time.Time
}

type SourceInfo struct {
	ID         string
	Name       string
	ChannelIDs []string
}

type GenerateReportInput struct {
	Date      string
	ChannelID string
	UserID    string
}

type GenerateSummaryInput struct {
	ClusterID string
}

type QwenClient interface {
	GenerateReport(context.Context, string) (string, error)
}

type ReportRepository interface {
	SaveReport(context.Context, DailyReport) (DailyReport, error)
	FindReportByDateChannelUser(context.Context, string, string, string) (DailyReport, error)
	FindReportByID(context.Context, string) (DailyReport, error)
	ListReportsByDate(context.Context, string) ([]DailyReport, error)
	SaveSummary(context.Context, AISummary) (AISummary, error)
	FindSummaryByClusterID(context.Context, string) (AISummary, error)
}

type ClusterRepository interface {
	ListClusters(context.Context) ([]ClusterInfo, error)
	ListClusterItems(context.Context, string) ([]ContentItemInfo, error)
}

type BatchClusterRepository interface {
	ListClusterItemsByClusterIDs(context.Context, []string) (map[string][]ContentItemInfo, error)
}

type ScoreRepository interface {
	ListScores(context.Context) ([]ScoreInfo, error)
}

type SourceRepository interface {
	ListSources(context.Context) ([]SourceInfo, error)
}

type PreferenceRepository interface {
	ListUserChannelIDs(context.Context, string) ([]string, error)
	ListUserKeywords(context.Context, string) ([]string, error)
}
