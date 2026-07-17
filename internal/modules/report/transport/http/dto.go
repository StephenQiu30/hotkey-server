package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
)

// ReportResult exists solely for Swagger's source parser. Runtime responses
// are written through the shared Result helpers.
type ReportResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type ReportItemResponse struct {
	EventID         int64   `json:"event_id"`
	Rank            int     `json:"rank"`
	InclusionReason string  `json:"inclusion_reason"`
	Title           string  `json:"title"`
	Summary         string  `json:"summary"`
	HeatScore       float64 `json:"heat_score"`
}

type ReportResponse struct {
	ID          int64                `json:"id"`
	Version     int64                `json:"version"`
	VersionNo   int64                `json:"version_no"`
	Type        string               `json:"type"`
	MonitorID   *int64               `json:"monitor_id,omitempty"`
	PeriodStart time.Time            `json:"period_start"`
	PeriodEnd   time.Time            `json:"period_end"`
	Timezone    string               `json:"timezone"`
	Title       string               `json:"title"`
	Summary     string               `json:"summary"`
	Body        string               `json:"body"`
	Status      string               `json:"status"`
	Frozen      bool                 `json:"frozen"`
	GeneratedAt *time.Time           `json:"generated_at,omitempty"`
	PublishedAt *time.Time           `json:"published_at,omitempty"`
	Items       []ReportItemResponse `json:"items"`
}

type ReportPageResponse struct {
	Items      []ReportResponse `json:"items"`
	NextCursor int64            `json:"next_cursor,omitempty"`
}

type ReportPreviewResponse struct {
	Report      ReportResponse `json:"report"`
	Publishable bool           `json:"publishable"`
}

func reportResponse(report domain.Report) ReportResponse {
	items := make([]ReportItemResponse, 0, len(report.Items))
	for _, item := range report.Items {
		items = append(items, ReportItemResponse{EventID: item.EventID, Rank: item.Rank, InclusionReason: item.InclusionReason, Title: item.Title, Summary: item.Summary, HeatScore: item.HeatScore})
	}
	timezone := ""
	if report.Period.Location != nil {
		timezone = report.Period.Location.String()
	}
	return ReportResponse{ID: report.ID, Version: report.Version, VersionNo: report.VersionNo, Type: string(report.Type), MonitorID: report.MonitorID, PeriodStart: report.Period.Start, PeriodEnd: report.Period.End, Timezone: timezone, Title: report.Title, Summary: report.Summary, Body: report.Body, Status: string(report.Status), Frozen: report.Frozen, GeneratedAt: report.GeneratedAt, PublishedAt: report.PublishedAt, Items: items}
}
