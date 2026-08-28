package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
)

// ReportResult exists solely for Swagger's source parser. Runtime responses
// are written through the shared Result helpers.
type ReportResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type CreateReportRequest struct {
	Type      string     `json:"type" binding:"required,oneof=daily weekly"`
	MonitorID *int64     `json:"monitor_id,omitempty"`
	Timezone  string     `json:"timezone" binding:"required"`
	At        *time.Time `json:"at,omitempty"`
}

type ReportRevisionLifecycleRequest struct {
	ExpectedResourceVersion int64  `json:"expected_resource_version" binding:"required,gt=0"`
	ReasonCode              string `json:"reason_code,omitempty"`
}

type ReportItemResponse struct {
	EventID             int64                    `json:"event_id,omitempty"`
	EventUpdateID       int64                    `json:"event_update_id,omitempty"`
	MicroEventID        int64                    `json:"micro_event_id,omitempty"`
	MicroEventVersion   int64                    `json:"micro_event_version,omitempty"`
	MicroEventUpdateID  int64                    `json:"micro_event_update_id,omitempty"`
	MicroEventSummaryID int64                    `json:"micro_event_summary_id,omitempty"`
	Rank                int                      `json:"rank"`
	InclusionReason     string                   `json:"inclusion_reason"`
	Title               string                   `json:"title"`
	Summary             string                   `json:"summary"`
	HeatScore           float64                  `json:"heat_score"`
	EvidenceSetHash     string                   `json:"evidence_set_hash"`
	ReasonCodes         []string                 `json:"reason_codes"`
	Sentences           []ReportSentenceResponse `json:"sentences"`
}

type ReportSentenceResponse struct {
	SourceSummarySentenceID int64   `json:"source_summary_sentence_id"`
	Ordinal                 int     `json:"ordinal"`
	Text                    string  `json:"text"`
	EditorialNote           bool    `json:"editorial_note"`
	DecisionOrigin          string  `json:"decision_origin"`
	ModelRunID              *int64  `json:"model_run_id,omitempty"`
	ActorUserID             *int64  `json:"actor_user_id,omitempty"`
	ClaimEvidenceVersionIDs []int64 `json:"claim_evidence_version_ids"`
}

type ReportResponse struct {
	ID                int64                `json:"id"`
	Version           int64                `json:"version"`
	VersionNo         int64                `json:"version_no"`
	Type              string               `json:"type"`
	MonitorID         *int64               `json:"monitor_id,omitempty"`
	PeriodStart       time.Time            `json:"period_start"`
	PeriodEnd         time.Time            `json:"period_end"`
	Timezone          string               `json:"timezone"`
	Title             string               `json:"title"`
	Summary           string               `json:"summary"`
	Body              string               `json:"body"`
	InputSnapshotHash string               `json:"input_snapshot_hash"`
	Status            string               `json:"status"`
	Frozen            bool                 `json:"frozen"`
	GeneratedAt       *time.Time           `json:"generated_at,omitempty"`
	PublishedAt       *time.Time           `json:"published_at,omitempty"`
	SubmittedAt       *time.Time           `json:"submitted_at,omitempty"`
	ReviewedAt        *time.Time           `json:"reviewed_at,omitempty"`
	CreatedBy         *int64               `json:"created_by,omitempty"`
	UpdatedBy         *int64               `json:"updated_by,omitempty"`
	SubmittedBy       *int64               `json:"submitted_by,omitempty"`
	ReviewedBy        *int64               `json:"reviewed_by,omitempty"`
	ReviewReason      string               `json:"review_reason,omitempty"`
	Items             []ReportItemResponse `json:"items"`
}

type ReportPageResponse struct {
	Items      []ReportResponse `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type ReportPreviewResponse struct {
	Report      ReportResponse `json:"report"`
	Submittable bool           `json:"submittable"`
	Approvable  bool           `json:"approvable"`
}

func reportResponse(report domain.Report) ReportResponse {
	items := make([]ReportItemResponse, 0, len(report.Items))
	for _, item := range report.Items {
		sentences := make([]ReportSentenceResponse, 0, len(item.Sentences))
		for _, sentence := range item.Sentences {
			sentences = append(sentences, ReportSentenceResponse{SourceSummarySentenceID: sentence.SourceSummarySentenceID,
				Ordinal: sentence.Ordinal, Text: sentence.Text, EditorialNote: sentence.EditorialNote,
				DecisionOrigin: sentence.DecisionOrigin, ModelRunID: sentence.ModelRunID, ActorUserID: sentence.ActorUserID,
				ClaimEvidenceVersionIDs: append([]int64(nil), sentence.ClaimEvidenceVersionIDs...)})
		}
		items = append(items, ReportItemResponse{EventID: item.EventID, EventUpdateID: item.EventUpdateID,
			MicroEventID: item.MicroEventID, MicroEventVersion: item.MicroEventVersion,
			MicroEventUpdateID: item.MicroEventUpdateID, MicroEventSummaryID: item.MicroEventSummaryID,
			Rank: item.Rank, InclusionReason: item.InclusionReason, Title: item.Title, Summary: item.Summary,
			HeatScore: item.HeatScore, EvidenceSetHash: item.EvidenceSetHash,
			ReasonCodes: append([]string(nil), item.ReasonCodes...), Sentences: sentences})
	}
	timezone := ""
	if report.Period.Location != nil {
		timezone = report.Period.Location.String()
	}
	return ReportResponse{ID: report.ID, Version: report.Version, VersionNo: report.VersionNo, Type: string(report.Type), MonitorID: report.MonitorID, PeriodStart: report.Period.Start, PeriodEnd: report.Period.End, Timezone: timezone, Title: report.Title, Summary: report.Summary, Body: report.Body, InputSnapshotHash: report.InputSnapshotHash, Status: string(report.Status), Frozen: report.Frozen, GeneratedAt: report.GeneratedAt, PublishedAt: report.PublishedAt, SubmittedAt: report.SubmittedAt, ReviewedAt: report.ReviewedAt, CreatedBy: report.CreatedBy, UpdatedBy: report.UpdatedBy, SubmittedBy: report.SubmittedBy, ReviewedBy: report.ReviewedBy, ReviewReason: report.ReviewReason, Items: items}
}
