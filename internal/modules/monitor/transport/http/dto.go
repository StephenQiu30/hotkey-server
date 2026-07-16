package http

import (
	"encoding/json"
	"fmt"

	monitorapplication "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
)

// MonitorResult mirrors the shared Result envelope only for swag's source
// parser. Runtime output always uses the platform HTTP result helpers.
type MonitorResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type MonitorConfigRequest struct {
	Timezone                  string   `json:"timezone" binding:"required" example:"Asia/Shanghai"`
	Languages                 []string `json:"languages" binding:"required,min=1" example:"en"`
	Regions                   []string `json:"regions"`
	CollectionIntervalSeconds int      `json:"collection_interval_seconds" binding:"required" example:"900"`
	RelevanceThreshold        float64  `json:"relevance_threshold" binding:"required" example:"75"`
	EventThreshold            float64  `json:"event_threshold" binding:"required" example:"40"`
	RetentionDays             int      `json:"retention_days" binding:"required" example:"30"`
}

type MonitorRuleRequest struct {
	RuleType string  `json:"rule_type" binding:"required" example:"keyword"`
	Operator string  `json:"operator" binding:"required" example:"contains"`
	Value    string  `json:"value" binding:"required" example:"OpenAI"`
	Weight   float64 `json:"weight"`
	Priority int16   `json:"priority"`
	Enabled  *bool   `json:"enabled,omitempty"`
}

type MonitorSourceRequest struct {
	SourceConnectionID int64  `json:"source_connection_id" binding:"required,gt=0"`
	QueryOverride      string `json:"query_override"`
	Priority           int16  `json:"priority"`
	Enabled            *bool  `json:"enabled,omitempty"`
}

type CreateMonitorRequest struct {
	Name        string                 `json:"name" binding:"required" example:"AI releases"`
	Description string                 `json:"description"`
	Config      MonitorConfigRequest   `json:"config" binding:"required"`
	Rules       []MonitorRuleRequest   `json:"rules" binding:"required,min=1"`
	Sources     []MonitorSourceRequest `json:"sources" binding:"required,min=1"`
}

// ExpectedDraftRequest uses RawMessage so omitted and explicit JSON null have
// distinct meanings. This is essential for the first-draft concurrency
// protocol; a missing field is never silently interpreted as null.
type ExpectedDraftRequest struct {
	ExpectedMonitorVersion int64                        `json:"expected_monitor_version" binding:"required,gt=0"`
	ExpectedDraftVersion   NullableExpectedDraftVersion `json:"expected_draft_version" swaggertype:"integer"`
}

// NullableExpectedDraftVersion retains both required JSON states: an explicit
// null starts a first draft, while an integer addresses an existing draft.
// Its unexported state also makes omission distinct from JSON null.
type NullableExpectedDraftVersion struct {
	value   *int64
	present bool
}

func (value *NullableExpectedDraftVersion) UnmarshalJSON(data []byte) error {
	value.present = true
	if string(data) == "null" {
		value.value = nil
		return nil
	}
	var parsed int64
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	value.value = &parsed
	return nil
}

type ReplaceDraftRequest struct {
	ExpectedDraftRequest
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Config      MonitorConfigRequest   `json:"config" binding:"required"`
	Rules       []MonitorRuleRequest   `json:"rules" binding:"required,min=1"`
	Sources     []MonitorSourceRequest `json:"sources" binding:"required,min=1"`
}

type AICandidateRequest struct {
	ExpectedDraftRequest
	RuleType string  `json:"rule_type" binding:"required"`
	Operator string  `json:"operator" binding:"required"`
	Value    string  `json:"value" binding:"required"`
	Weight   float64 `json:"weight"`
	Priority int16   `json:"priority"`
}

type ApprovalRequest struct {
	ExpectedDraftRequest
	Approval string `json:"approval" binding:"required,oneof=approved rejected"`
}

type PublishRequest struct{ ExpectedDraftRequest }

type LifecycleRequest struct {
	ExpectedMonitorVersion int64 `json:"expected_monitor_version" binding:"required,gt=0"`
}

type MonitorRuleResponse struct {
	ID             int64   `json:"id"`
	RuleType       string  `json:"rule_type"`
	Operator       string  `json:"operator"`
	Value          string  `json:"value"`
	Weight         float64 `json:"weight"`
	Priority       int16   `json:"priority"`
	Origin         string  `json:"origin"`
	ApprovalStatus string  `json:"approval_status"`
	Enabled        bool    `json:"enabled"`
}

type MonitorSourceResponse struct {
	ID                 int64  `json:"id"`
	SourceConnectionID int64  `json:"source_connection_id"`
	QueryOverride      string `json:"query_override"`
	Priority           int16  `json:"priority"`
	Enabled            bool   `json:"enabled"`
}

type MonitorConfigResponse struct {
	ID                        int64                   `json:"id"`
	Version                   int64                   `json:"version"`
	Revision                  int64                   `json:"revision"`
	Timezone                  string                  `json:"timezone"`
	Languages                 []string                `json:"languages"`
	Regions                   []string                `json:"regions"`
	CollectionIntervalSeconds int                     `json:"collection_interval_seconds"`
	RelevanceThreshold        float64                 `json:"relevance_threshold"`
	EventThreshold            float64                 `json:"event_threshold"`
	RetentionDays             int                     `json:"retention_days"`
	Rules                     []MonitorRuleResponse   `json:"rules"`
	Sources                   []MonitorSourceResponse `json:"sources"`
}

type MonitorResponse struct {
	ID                int64                  `json:"id"`
	Version           int64                  `json:"version"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	Status            string                 `json:"status"`
	PublishedRevision *int64                 `json:"published_revision,omitempty"`
	Published         *MonitorConfigResponse `json:"published,omitempty"`
	Draft             *MonitorConfigResponse `json:"draft,omitempty"`
}

type MonitorPageResponse struct {
	Items      []MonitorResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

type PreviewSourceResponse struct {
	SourceConnectionID int64   `json:"source_connection_id"`
	QuerySignature     string  `json:"query_signature"`
	IncludedRuleIDs    []int64 `json:"included_rule_ids"`
	ExcludedRuleIDs    []int64 `json:"excluded_rule_ids"`
	UnapprovedRuleIDs  []int64 `json:"unapproved_rule_ids"`
	EstimatedRequests  int     `json:"estimated_requests"`
}

type PreviewResponse struct {
	Eligible          bool                    `json:"eligible"`
	ConfigHash        string                  `json:"config_hash"`
	Sources           []PreviewSourceResponse `json:"sources"`
	EstimatedRequests int                     `json:"estimated_requests"`
	Warnings          []string                `json:"warnings"`
}

func expectedVersions(request ExpectedDraftRequest) (domain.ExpectedVersions, error) {
	if request.ExpectedMonitorVersion <= 0 || !request.ExpectedDraftVersion.present {
		return domain.ExpectedVersions{}, fmt.Errorf("expected monitor and explicit draft versions are required")
	}
	if request.ExpectedDraftVersion.value == nil {
		return domain.ExpectedVersions{MonitorVersion: request.ExpectedMonitorVersion, DraftVersion: nil}, nil
	}
	value := *request.ExpectedDraftVersion.value
	if value <= 0 {
		return domain.ExpectedVersions{}, fmt.Errorf("expected_draft_version must be a positive integer or null")
	}
	return domain.ExpectedVersions{MonitorVersion: request.ExpectedMonitorVersion, DraftVersion: &value}, nil
}

func monitorDraft(request CreateMonitorRequest) monitorapplication.DraftInput {
	return monitorapplication.DraftInput{Name: request.Name, Description: request.Description, Config: monitorConfig(request.Config), Rules: monitorRules(request.Rules), Sources: monitorSources(request.Sources)}
}

func replaceMonitorDraft(request ReplaceDraftRequest) monitorapplication.DraftInput {
	return monitorapplication.DraftInput{Name: request.Name, Description: request.Description, Config: monitorConfig(request.Config), Rules: monitorRules(request.Rules), Sources: monitorSources(request.Sources)}
}

func monitorConfig(request MonitorConfigRequest) domain.MonitorConfig {
	return domain.MonitorConfig{Timezone: request.Timezone, Languages: request.Languages, Regions: request.Regions, CollectionIntervalSeconds: request.CollectionIntervalSeconds, RelevanceThreshold: request.RelevanceThreshold, EventThreshold: request.EventThreshold, RetentionDays: request.RetentionDays}
}

func monitorRules(requests []MonitorRuleRequest) []domain.MonitorRule {
	rules := make([]domain.MonitorRule, 0, len(requests))
	for _, request := range requests {
		enabled := true
		if request.Enabled != nil {
			enabled = *request.Enabled
		}
		rules = append(rules, domain.MonitorRule{RuleType: domain.RuleType(request.RuleType), Operator: domain.RuleOperator(request.Operator), Value: request.Value, Weight: request.Weight, Priority: request.Priority, Origin: domain.RuleOriginUser, ApprovalStatus: domain.RuleApprovalApproved, Enabled: enabled})
	}
	return rules
}

func monitorSources(requests []MonitorSourceRequest) []domain.MonitorSource {
	sources := make([]domain.MonitorSource, 0, len(requests))
	for _, request := range requests {
		enabled := true
		if request.Enabled != nil {
			enabled = *request.Enabled
		}
		sources = append(sources, domain.MonitorSource{SourceConnectionID: request.SourceConnectionID, QueryOverride: request.QueryOverride, Priority: request.Priority, Enabled: enabled})
	}
	return sources
}

func monitorResponse(monitor domain.Monitor, config *domain.MonitorConfigVersion, rules []domain.MonitorRule, sources []domain.MonitorSource, draft bool) MonitorResponse {
	response := MonitorResponse{ID: monitor.ID, Version: monitor.Version, Name: monitor.Name, Description: monitor.Description, Status: string(monitor.Status)}
	if config == nil {
		return response
	}
	view := monitorConfigResponse(*config, rules, sources)
	if draft {
		response.Draft = &view
	} else {
		response.Published = &view
		revision := config.Revision
		response.PublishedRevision = &revision
	}
	return response
}

func monitorConfigResponse(config domain.MonitorConfigVersion, rules []domain.MonitorRule, sources []domain.MonitorSource) MonitorConfigResponse {
	response := MonitorConfigResponse{ID: config.ID, Version: config.Version, Revision: config.Revision, Timezone: config.Config.Timezone, Languages: config.Config.Languages, Regions: config.Config.Regions, CollectionIntervalSeconds: config.Config.CollectionIntervalSeconds, RelevanceThreshold: config.Config.RelevanceThreshold, EventThreshold: config.Config.EventThreshold, RetentionDays: config.Config.RetentionDays, Rules: make([]MonitorRuleResponse, 0, len(rules)), Sources: make([]MonitorSourceResponse, 0, len(sources))}
	for _, rule := range rules {
		response.Rules = append(response.Rules, MonitorRuleResponse{ID: rule.ID, RuleType: string(rule.RuleType), Operator: string(rule.Operator), Value: rule.Value, Weight: rule.Weight, Priority: rule.Priority, Origin: string(rule.Origin), ApprovalStatus: string(rule.ApprovalStatus), Enabled: rule.Enabled})
	}
	for _, source := range sources {
		response.Sources = append(response.Sources, MonitorSourceResponse{ID: source.ID, SourceConnectionID: source.SourceConnectionID, QueryOverride: source.QueryOverride, Priority: source.Priority, Enabled: source.Enabled})
	}
	return response
}

func previewResponse(preview monitorapplication.PreviewResult) PreviewResponse {
	response := PreviewResponse{Eligible: preview.Eligible, ConfigHash: preview.ConfigHash, Sources: make([]PreviewSourceResponse, 0, len(preview.Sources)), Warnings: preview.Warnings}
	for _, source := range preview.Sources {
		response.Sources = append(response.Sources, PreviewSourceResponse{SourceConnectionID: source.SourceConnectionID, QuerySignature: source.QuerySignature, IncludedRuleIDs: source.IncludedRuleIDs, ExcludedRuleIDs: source.ExcludedRuleIDs, UnapprovedRuleIDs: source.UnapprovedRuleIDs, EstimatedRequests: source.EstimatedRequests})
		response.EstimatedRequests += source.EstimatedRequests
	}
	return response
}
