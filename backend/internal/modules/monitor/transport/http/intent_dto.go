package http

import "time"

type IntentClauseRequestDTO struct {
	Operator string `json:"operator" binding:"required,oneof=must should must_not" enums:"must,should,must_not"`
	Field    string `json:"field" binding:"required,oneof=term phrase action location language region source time_window" enums:"term,phrase,action,location,language,region,source,time_window"`
	Value    string `json:"value" binding:"required"`
}

type IntentEntityRequestDTO struct {
	CanonicalID   string   `json:"canonical_id" binding:"required"`
	DisplayName   string   `json:"display_name" binding:"required"`
	Aliases       []string `json:"aliases" binding:"max=32" maxItems:"32"`
	AmbiguityNote string   `json:"ambiguity_note"`
}

type IntentExampleRequestDTO struct {
	Label string `json:"label" binding:"required,oneof=positive negative" enums:"positive,negative"`
	Text  string `json:"text" binding:"required"`
}

type ReplaceIntentDraftRequestDTO struct {
	ExpectedResourceVersion int64                     `json:"expected_resource_version" binding:"gte=0" minimum:"0"`
	Objective               string                    `json:"objective" binding:"required"`
	Clauses                 []IntentClauseRequestDTO  `json:"clauses" binding:"max=128,dive" maxItems:"128"`
	Entities                []IntentEntityRequestDTO  `json:"entities" binding:"max=64,dive" maxItems:"64"`
	Examples                []IntentExampleRequestDTO `json:"examples" binding:"max=64,dive" maxItems:"64"`
}

type SubmitIntentExpansionRunRequestDTO struct {
	ExpectedResourceVersion int64  `json:"expected_resource_version" binding:"required,gt=0" minimum:"1"`
	ExpansionProfile        string `json:"expansion_profile" binding:"required,oneof=monitor-intent-expansion-v1" enums:"monitor-intent-expansion-v1" example:"monitor-intent-expansion-v1"`
}

type SubmitIntentPreviewRunRequestDTO struct {
	ExpectedResourceVersion int64  `json:"expected_resource_version" binding:"required,gt=0" minimum:"1"`
	EvaluatorProfile        string `json:"evaluator_profile" binding:"required"`
	SampleLimit             int    `json:"sample_limit" binding:"required,gte=1,lte=200" minimum:"1" maximum:"200"`
}

type ReviewIntentExpansionCandidateRequestDTO struct {
	ExpectedResourceVersion int64  `json:"expected_resource_version" binding:"required,gt=0" minimum:"1"`
	Decision                string `json:"decision" binding:"required,oneof=approved rejected" enums:"approved,rejected"`
	Note                    string `json:"note"`
}

type IntentClauseResponseDTO struct {
	Operator string `json:"operator"`
	Field    string `json:"field"`
	Value    string `json:"value"`
}

type IntentEntityResponseDTO struct {
	CanonicalID   string   `json:"canonical_id"`
	DisplayName   string   `json:"display_name"`
	Aliases       []string `json:"aliases"`
	AmbiguityNote string   `json:"ambiguity_note"`
}

type IntentExampleResponseDTO struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

// IntentExpansionCandidateResponseDTO exposes provenance versions but cannot
// represent prompt content, raw provider output, or document body text.
type IntentExpansionCandidateResponseDTO struct {
	ID             string     `json:"id"`
	Value          string     `json:"value"`
	Source         string     `json:"source"`
	Reason         string     `json:"reason"`
	ModelVersion   string     `json:"model_version"`
	PromptVersion  string     `json:"prompt_version"`
	InputHash      string     `json:"input_hash"`
	Similarity     float64    `json:"similarity"`
	Risk           string     `json:"risk"`
	ApprovalStatus string     `json:"approval_status"`
	ReviewerUserID *int64     `json:"reviewer_user_id,omitempty" extensions:"x-nullable"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty" extensions:"x-nullable"`
	ReviewNote     string     `json:"review_note,omitempty"`
}

type IntentDraftResponseDTO struct {
	MonitorID       int64                                 `json:"monitor_id"`
	DraftID         int64                                 `json:"draft_id"`
	ResourceVersion int64                                 `json:"resource_version"`
	Objective       string                                `json:"objective"`
	Clauses         []IntentClauseResponseDTO             `json:"clauses"`
	Entities        []IntentEntityResponseDTO             `json:"entities"`
	Examples        []IntentExampleResponseDTO            `json:"examples"`
	Candidates      []IntentExpansionCandidateResponseDTO `json:"candidates"`
}

type IntentRunAcceptedResponseDTO struct {
	RunID           int64  `json:"run_id"`
	Kind            string `json:"kind" enums:"expansion,preview"`
	MonitorID       int64  `json:"monitor_id"`
	DraftID         int64  `json:"draft_id"`
	ResourceVersion int64  `json:"resource_version"`
	InputHash       string `json:"input_hash"`
	Status          string `json:"status"`
	StatusURL       string `json:"status_url"`
	Reused          bool   `json:"reused"`
}

type IntentAnalysisRunResponseDTO struct {
	RunID           int64      `json:"run_id"`
	Kind            string     `json:"kind" enums:"expansion,preview"`
	MonitorID       int64      `json:"monitor_id"`
	DraftID         int64      `json:"draft_id"`
	ResourceVersion int64      `json:"resource_version"`
	InputHash       string     `json:"input_hash"`
	Status          string     `json:"status" enums:"queued,running,succeeded,failed,invalidated"`
	StatusURL       string     `json:"status_url"`
	QueuedAt        time.Time  `json:"queued_at"`
	StartedAt       *time.Time `json:"started_at,omitempty" extensions:"x-nullable"`
	CompletedAt     *time.Time `json:"completed_at,omitempty" extensions:"x-nullable"`
	InvalidatedAt   *time.Time `json:"invalidated_at,omitempty" extensions:"x-nullable"`
	FailureCode     string     `json:"failure_code,omitempty"`
}

type IntentExpansionRunStatusResponseDTO struct {
	IntentAnalysisRunResponseDTO
	Candidates []IntentExpansionCandidateResponseDTO `json:"candidates"`
}

type IntentPreviewRecallSignalResponseDTO struct {
	Channel string `json:"channel"`
	Rank    int    `json:"rank"`
	// RawScore is comparable only within the same recall channel. It is not a
	// probability or a cross-channel relevance percentage.
	RawScore float64 `json:"raw_score"`
}

type IntentPreviewSampleResponseDTO struct {
	DocumentVersionID int64                                  `json:"document_version_id"`
	Title             string                                 `json:"title"`
	Decision          string                                 `json:"decision"`
	RecallSignals     []IntentPreviewRecallSignalResponseDTO `json:"recall_signals"`
	Reasons           []string                               `json:"reasons"`
	ExclusionReasons  []string                               `json:"exclusion_reasons"`
}

type IntentPreviewResponseDTO struct {
	Samples             []IntentPreviewSampleResponseDTO `json:"samples"`
	EstimatedAlertCount int                              `json:"estimated_alert_count"`
	Warnings            []string                         `json:"warnings"`
}

type IntentPreviewRunStatusResponseDTO struct {
	IntentAnalysisRunResponseDTO
	Preview *IntentPreviewResponseDTO `json:"preview,omitempty" extensions:"x-nullable"`
}
