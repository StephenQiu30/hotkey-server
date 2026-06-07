package eventsummary

import (
	"context"
	"errors"
	"time"
)

const PromptVersion = "event_summary_zh_v1"

type ModelStatus string

const (
	ModelStatusPending   ModelStatus = "pending"
	ModelStatusSucceeded ModelStatus = "succeeded"
	ModelStatusFailed    ModelStatus = "failed"
	ModelStatusDegraded  ModelStatus = "degraded"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
)

type TimelineEntry struct {
	Date        string `json:"date"`
	Description string `json:"description"`
}

type SourceRef struct {
	SourceID string `json:"sourceId"`
	ItemID   string `json:"itemId"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

type EventSummary struct {
	ID            string          `json:"id"`
	EventID       string          `json:"eventId"`
	PromptVersion string          `json:"promptVersion"`
	Title         string          `json:"title"`
	Summary       string          `json:"summary"`
	Timeline      []TimelineEntry `json:"timeline"`
	KeySignals    []string        `json:"keySignals"`
	SourceRefs    []SourceRef     `json:"sourceRefs"`
	RiskAlerts    []string        `json:"riskAlerts"`
	FollowUp      []string        `json:"followUp"`
	Confidence    float64         `json:"confidence"`
	ModelStatus   ModelStatus     `json:"modelStatus"`
	LastError     string          `json:"lastError,omitempty"`
	Version       int             `json:"version"`
	LowEvidence   bool            `json:"lowEvidence"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type ItemInfo struct {
	ID       string
	SourceID string
	Title    string
	Snippet  string
	URL      string
}

type GenerateSummaryInput struct {
	EventID string
	Title   string
	Items   []ItemInfo
}

type QwenClient interface {
	GenerateReport(context.Context, string) (string, error)
}

type SummaryRepository interface {
	Save(context.Context, EventSummary) (EventSummary, error)
	FindByEventID(context.Context, string) (EventSummary, error)
}
