package http

import (
	"encoding/json"
	"fmt"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
)

// MonitorResult mirrors the shared Result envelope only for swag's source
// parser. Runtime output always uses the platform HTTP result helpers.
type MonitorResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type MonitorQuotaErrorResponse struct {
	Code    int    `json:"code" example:"10005"`
	Message string `json:"message" example:"product quota exceeded"`
	Data    struct {
		Dimension string  `json:"dimension" example:"active_monitors"`
		Limit     int64   `json:"limit" example:"50"`
		Remaining int64   `json:"remaining" example:"0"`
		ResetAt   *string `json:"reset_at"`
	} `json:"data"`
}

type MonitorConfigRequest struct {
	Timezone                  string   `json:"timezone" binding:"required" example:"Asia/Shanghai"`
	Languages                 []string `json:"languages" binding:"required,min=1,max=8" minItems:"1" maxItems:"8" example:"en"`
	Regions                   []string `json:"regions" binding:"max=8" maxItems:"8"`
	CollectionIntervalSeconds int      `json:"collection_interval_seconds" binding:"required,gte=300,lte=86400" minimum:"300" maximum:"86400" example:"900"`
	RelevanceThreshold        float64  `json:"relevance_threshold" binding:"required,gte=60,lte=100" minimum:"60" maximum:"100" example:"75"`
	EventThreshold            *float64 `json:"event_threshold" binding:"required,gte=0,lte=100" minimum:"0" maximum:"100" example:"40"`
	AlertMinHeat              *float64 `json:"alert_min_heat,omitempty" binding:"omitempty,gte=0,lte=100" minimum:"0" maximum:"100" example:"70"`
	AlertMinMomentum          *float64 `json:"alert_min_momentum,omitempty" binding:"omitempty,gte=0,lte=100" minimum:"0" maximum:"100" example:"55"`
	AlertMinBreadth           *float64 `json:"alert_min_breadth,omitempty" binding:"omitempty,gte=0,lte=100" minimum:"0" maximum:"100" example:"25"`
	AlertWarningThreshold     *float64 `json:"alert_warning_threshold,omitempty" binding:"omitempty,gte=0,lte=100" minimum:"0" maximum:"100" example:"75"`
	AlertCriticalThreshold    *float64 `json:"alert_critical_threshold,omitempty" binding:"omitempty,gte=0,lte=100" minimum:"0" maximum:"100" example:"90"`
	AlertCooldownMinutes      *int     `json:"alert_cooldown_minutes,omitempty" binding:"omitempty,gte=5,lte=1440" minimum:"5" maximum:"1440" example:"60"`
	AlertEmailEnabled         *bool    `json:"alert_email_enabled,omitempty" example:"false"`
	AlertEmailMinSeverity     string   `json:"alert_email_min_severity,omitempty" binding:"omitempty,oneof=warning critical" enums:"warning,critical" example:"critical"`
	RetentionDays             int      `json:"retention_days" binding:"required,gte=1,lte=3650" minimum:"1" maximum:"3650" example:"30"`
}

type MonitorRuleRequest struct {
	RuleType string  `json:"rule_type" binding:"required" example:"keyword"`
	Operator string  `json:"operator" binding:"required" example:"contains"`
	Value    string  `json:"value" binding:"required" example:"OpenAI"`
	Weight   float64 `json:"weight"`
	Priority *int16  `json:"priority,omitempty" default:"100"`
	Enabled  *bool   `json:"enabled,omitempty"`
}

type MonitorSourceRequest struct {
	SourceConnectionID int64  `json:"source_connection_id" binding:"required,gt=0"`
	QueryOverride      string `json:"query_override"`
	Priority           *int16 `json:"priority,omitempty" default:"100"`
	Enabled            *bool  `json:"enabled,omitempty"`
}

type CreateMonitorRequest struct {
	Name                      string  `json:"name" binding:"required,max=120" example:"AI 产品"`
	Query                     string  `json:"query" binding:"required,max=160" example:"Claude"`
	SourceConnectionIDs       []int64 `json:"source_connection_ids" binding:"required,min=1,max=10,dive,gt=0" minItems:"1" maxItems:"10"`
	CollectionIntervalSeconds int     `json:"collection_interval_seconds,omitempty" binding:"omitempty,gte=300,lte=86400" minimum:"300" maximum:"86400" default:"1800" example:"1800"`
	AlertEmailEnabled         *bool   `json:"alert_email_enabled,omitempty" default:"true" example:"true"`
}

type UpdateMonitorRequest struct {
	ExpectedMonitorVersion    int64   `json:"expected_monitor_version" binding:"required,gt=0"`
	Name                      string  `json:"name" binding:"required,max=120" example:"AI 产品"`
	Query                     string  `json:"query" binding:"required,max=160" example:"Claude"`
	SourceConnectionIDs       []int64 `json:"source_connection_ids" binding:"required,min=1,max=10,dive,gt=0" minItems:"1" maxItems:"10"`
	CollectionIntervalSeconds int     `json:"collection_interval_seconds" binding:"required,gte=300,lte=86400" minimum:"300" maximum:"86400" example:"1800"`
	AlertEmailEnabled         *bool   `json:"alert_email_enabled" binding:"required"`
}

// ExpectedDraftRequest uses RawMessage so omitted and explicit JSON null have
// distinct meanings. This is essential for the first-draft concurrency
// protocol; a missing field is never silently interpreted as null.
type ExpectedDraftRequest struct {
	ExpectedMonitorVersion int64 `json:"expected_monitor_version" binding:"required,gt=0"`
	// Gin must not apply required directly to this nullable wrapper: both an
	// explicit JSON null and a positive integer are valid. The application
	// helper below enforces presence/value at runtime; validate keeps Swagger's
	// required property without making explicit null impossible to bind.
	ExpectedDraftVersion NullableExpectedDraftVersion `json:"expected_draft_version" validate:"required" swaggertype:"integer" extensions:"x-nullable"`
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
	Rules       []MonitorRuleRequest   `json:"rules" binding:"required,min=1,max=100" minItems:"1" maxItems:"100"`
	Sources     []MonitorSourceRequest `json:"sources" binding:"required,min=1,max=10" minItems:"1" maxItems:"10"`
}

type AICandidateRequest struct {
	ExpectedDraftRequest
	RuleType string  `json:"rule_type" binding:"required"`
	Operator string  `json:"operator" binding:"required"`
	Value    string  `json:"value" binding:"required"`
	Weight   float64 `json:"weight"`
	Priority *int16  `json:"priority,omitempty" default:"100"`
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
	SourceConnectionID int64  `json:"source_connection_id"`
	Name               string `json:"name"`
	SourceType         string `json:"source_type"`
	Enabled            bool   `json:"enabled"`
}

type MonitorConfigResponse struct {
	ID                        int64                   `json:"id"`
	Version                   int64                   `json:"version"`
	Revision                  int64                   `json:"revision"`
	State                     string                  `json:"state"`
	ConfigHash                string                  `json:"config_hash"`
	PublishedAt               *time.Time              `json:"published_at,omitempty"`
	Timezone                  string                  `json:"timezone"`
	Languages                 []string                `json:"languages"`
	Regions                   []string                `json:"regions"`
	CollectionIntervalSeconds int                     `json:"collection_interval_seconds"`
	RelevanceThreshold        float64                 `json:"relevance_threshold"`
	EventThreshold            float64                 `json:"event_threshold"`
	AlertMinHeat              float64                 `json:"alert_min_heat"`
	AlertMinMomentum          float64                 `json:"alert_min_momentum"`
	AlertMinBreadth           float64                 `json:"alert_min_breadth"`
	AlertWarningThreshold     float64                 `json:"alert_warning_threshold"`
	AlertCriticalThreshold    float64                 `json:"alert_critical_threshold"`
	AlertCooldownMinutes      int                     `json:"alert_cooldown_minutes"`
	AlertEmailEnabled         bool                    `json:"alert_email_enabled"`
	AlertEmailMinSeverity     string                  `json:"alert_email_min_severity"`
	RetentionDays             int                     `json:"retention_days"`
	Rules                     []MonitorRuleResponse   `json:"rules"`
	Sources                   []MonitorSourceResponse `json:"sources"`
}

type MonitorVersionHistoryResponse struct {
	Items []MonitorConfigResponse `json:"items"`
}

type MonitorResponse struct {
	ID                        int64                   `json:"id"`
	Version                   int64                   `json:"version"`
	CreatedByUserID           int64                   `json:"created_by_user_id"`
	Name                      string                  `json:"name"`
	Description               string                  `json:"description"`
	Status                    string                  `json:"status"`
	Query                     string                  `json:"query"`
	CollectionIntervalSeconds int                     `json:"collection_interval_seconds"`
	AlertEmailEnabled         bool                    `json:"alert_email_enabled"`
	Sources                   []MonitorSourceResponse `json:"sources"`
}

type MonitorPageResponse struct {
	Items      []MonitorResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

type PreviewSourceResponse struct {
	SourceConnectionID int64    `json:"source_connection_id"`
	QuerySignature     string   `json:"query_signature"`
	CompiledQuery      string   `json:"compiled_query"`
	QueryMode          string   `json:"query_mode"`
	Languages          []string `json:"languages"`
	Regions            []string `json:"regions"`
	MaxQueryBytes      int      `json:"max_query_bytes"`
	IncludedRuleIDs    []int64  `json:"included_rule_ids"`
	ExcludedRuleIDs    []int64  `json:"excluded_rule_ids"`
	UnapprovedRuleIDs  []int64  `json:"unapproved_rule_ids"`
	IncludedTermCount  int      `json:"included_term_count"`
	ExcludedTermCount  int      `json:"excluded_term_count"`
	EstimatedRequests  int      `json:"estimated_requests"`
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
	interval := request.CollectionIntervalSeconds
	if interval == 0 {
		interval = 1800
	}
	emailEnabled := true
	if request.AlertEmailEnabled != nil {
		emailEnabled = *request.AlertEmailEnabled
	}
	return simpleMonitorDraft(simpleMonitorInput{
		Name: request.Name, Query: request.Query, SourceConnectionIDs: request.SourceConnectionIDs,
		CollectionIntervalSeconds: interval, AlertEmailEnabled: emailEnabled,
	})
}

func updateMonitorDraft(request UpdateMonitorRequest) monitorapplication.DraftInput {
	return simpleMonitorDraft(simpleMonitorInput{
		Name: request.Name, Query: request.Query, SourceConnectionIDs: request.SourceConnectionIDs,
		CollectionIntervalSeconds: request.CollectionIntervalSeconds,
		AlertEmailEnabled:         request.AlertEmailEnabled != nil && *request.AlertEmailEnabled,
	})
}

type simpleMonitorInput struct {
	Name                      string
	Query                     string
	SourceConnectionIDs       []int64
	CollectionIntervalSeconds int
	AlertEmailEnabled         bool
}

func simpleMonitorDraft(input simpleMonitorInput) monitorapplication.DraftInput {
	config := domain.DefaultMonitorAlertPolicy()
	config.Timezone = "Asia/Shanghai"
	config.Languages = []string{"zh", "en"}
	config.CollectionIntervalSeconds = input.CollectionIntervalSeconds
	config.RelevanceThreshold = 60
	config.AlertEmailEnabled = input.AlertEmailEnabled
	config.AlertEmailMinSeverity = domain.AlertEmailSeverityWarning
	config.RetentionDays = 30
	rules := []domain.MonitorRule{{
		RuleType: domain.RuleTypeKeyword, Operator: domain.RuleOperatorContains,
		Value: input.Query, Weight: 100, Priority: 1, Enabled: true,
	}}
	sources := make([]domain.MonitorSource, 0, len(input.SourceConnectionIDs))
	for index, sourceConnectionID := range input.SourceConnectionIDs {
		sources = append(sources, domain.MonitorSource{
			SourceConnectionID: sourceConnectionID, Priority: int16(index + 1), Enabled: true,
		})
	}
	return monitorapplication.DraftInput{
		Name: input.Name, Description: "监控 " + input.Query,
		Config: config,
		Rules:  rules, Sources: sources,
	}
}

func replaceMonitorDraft(request ReplaceDraftRequest) monitorapplication.DraftInput {
	return monitorapplication.DraftInput{Name: request.Name, Description: request.Description, Config: monitorConfig(request.Config), Rules: monitorRules(request.Rules), Sources: monitorSources(request.Sources)}
}

func monitorConfig(request MonitorConfigRequest) domain.MonitorConfig {
	eventThreshold := float64(0)
	if request.EventThreshold != nil {
		eventThreshold = *request.EventThreshold
	}
	alertMinHeat, alertMinMomentum, alertMinBreadth := float64(0), float64(0), float64(0)
	alertWarning, alertCritical, alertEmailEnabled := float64(0), float64(0), false
	if request.AlertMinHeat != nil {
		alertMinHeat = *request.AlertMinHeat
	}
	if request.AlertMinMomentum != nil {
		alertMinMomentum = *request.AlertMinMomentum
	}
	if request.AlertMinBreadth != nil {
		alertMinBreadth = *request.AlertMinBreadth
	}
	if request.AlertWarningThreshold != nil {
		alertWarning = *request.AlertWarningThreshold
	}
	if request.AlertCriticalThreshold != nil {
		alertCritical = *request.AlertCriticalThreshold
	}
	if request.AlertEmailEnabled != nil {
		alertEmailEnabled = *request.AlertEmailEnabled
	}
	alertCooldown := 0
	if request.AlertCooldownMinutes != nil {
		alertCooldown = *request.AlertCooldownMinutes
	}
	return domain.MonitorConfig{Timezone: request.Timezone, Languages: request.Languages, Regions: request.Regions, CollectionIntervalSeconds: request.CollectionIntervalSeconds, RelevanceThreshold: request.RelevanceThreshold, EventThreshold: eventThreshold, AlertMinHeat: alertMinHeat, AlertMinMomentum: alertMinMomentum, AlertMinBreadth: alertMinBreadth, AlertWarningThreshold: alertWarning, AlertCriticalThreshold: alertCritical, AlertCooldownMinutes: alertCooldown, AlertEmailEnabled: alertEmailEnabled, AlertEmailMinSeverity: domain.AlertEmailSeverity(request.AlertEmailMinSeverity), RetentionDays: request.RetentionDays}
}

func monitorRules(requests []MonitorRuleRequest) []domain.MonitorRule {
	rules := make([]domain.MonitorRule, 0, len(requests))
	for _, request := range requests {
		enabled := true
		if request.Enabled != nil {
			enabled = *request.Enabled
		}
		rules = append(rules, domain.MonitorRule{RuleType: domain.RuleType(request.RuleType), Operator: domain.RuleOperator(request.Operator), Value: request.Value, Weight: request.Weight, Priority: monitorPriority(request.Priority), Origin: domain.RuleOriginUser, ApprovalStatus: domain.RuleApprovalApproved, Enabled: enabled})
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
		sources = append(sources, domain.MonitorSource{SourceConnectionID: request.SourceConnectionID, QueryOverride: request.QueryOverride, Priority: monitorPriority(request.Priority), Enabled: enabled})
	}
	return sources
}

func aiCandidateRule(request AICandidateRequest) domain.MonitorRule {
	return domain.MonitorRule{RuleType: domain.RuleType(request.RuleType), Operator: domain.RuleOperator(request.Operator), Value: request.Value, Weight: request.Weight, Priority: monitorPriority(request.Priority)}
}

func monitorPriority(priority *int16) int16 {
	if priority == nil {
		return 100
	}
	return *priority
}

func monitorResponse(view monitorapplication.MonitorView) MonitorResponse {
	response := MonitorResponse{
		ID: view.Monitor.ID, Version: view.Monitor.Version, Name: view.Monitor.Name,
		CreatedByUserID: view.Monitor.CreatedByUserID,
		Description:     view.Monitor.Description, Status: string(view.Monitor.Status),
		Sources: []MonitorSourceResponse{},
	}
	configuration := view.Published
	if configuration == nil {
		configuration = view.Draft
	}
	if configuration == nil {
		return response
	}
	response.CollectionIntervalSeconds = configuration.Config.Config.CollectionIntervalSeconds
	response.AlertEmailEnabled = configuration.Config.Config.AlertEmailEnabled
	for _, rule := range configuration.Rules {
		if rule.Enabled && rule.ApprovalStatus == domain.RuleApprovalApproved && rule.RuleType != domain.RuleTypeExcludeKeyword {
			response.Query = rule.Value
			break
		}
	}
	for _, source := range configuration.Sources {
		if !source.MonitorSource.Enabled {
			continue
		}
		response.Sources = append(response.Sources, MonitorSourceResponse{
			SourceConnectionID: source.MonitorSource.SourceConnectionID,
			Name:               source.SourceName, SourceType: source.SourceType, Enabled: true,
		})
	}
	return response
}

func monitorConfigResponse(view monitorapplication.ConfigurationView) MonitorConfigResponse {
	config, rules, sources := view.Config, view.Rules, view.Sources
	response := MonitorConfigResponse{ID: config.ID, Version: config.Version, Revision: config.Revision, State: string(config.State), ConfigHash: config.ConfigHash, PublishedAt: config.PublishedAt, Timezone: config.Config.Timezone, Languages: config.Config.Languages, Regions: config.Config.Regions, CollectionIntervalSeconds: config.Config.CollectionIntervalSeconds, RelevanceThreshold: config.Config.RelevanceThreshold, EventThreshold: config.Config.EventThreshold, AlertMinHeat: config.Config.AlertMinHeat, AlertMinMomentum: config.Config.AlertMinMomentum, AlertMinBreadth: config.Config.AlertMinBreadth, AlertWarningThreshold: config.Config.AlertWarningThreshold, AlertCriticalThreshold: config.Config.AlertCriticalThreshold, AlertCooldownMinutes: config.Config.AlertCooldownMinutes, AlertEmailEnabled: config.Config.AlertEmailEnabled, AlertEmailMinSeverity: string(config.Config.AlertEmailMinSeverity), RetentionDays: config.Config.RetentionDays, Rules: make([]MonitorRuleResponse, 0, len(rules)), Sources: make([]MonitorSourceResponse, 0, len(sources))}
	for _, rule := range rules {
		response.Rules = append(response.Rules, MonitorRuleResponse{ID: rule.ID, RuleType: string(rule.RuleType), Operator: string(rule.Operator), Value: rule.Value, Weight: rule.Weight, Priority: rule.Priority, Origin: string(rule.Origin), ApprovalStatus: string(rule.ApprovalStatus), Enabled: rule.Enabled})
	}
	for _, source := range sources {
		response.Sources = append(response.Sources, MonitorSourceResponse{SourceConnectionID: source.MonitorSource.SourceConnectionID, Name: source.SourceName, SourceType: source.SourceType, Enabled: source.MonitorSource.Enabled})
	}
	return response
}

func previewResponse(preview monitorapplication.PreviewResult) PreviewResponse {
	response := PreviewResponse{Eligible: preview.Eligible, ConfigHash: preview.ConfigHash, Sources: make([]PreviewSourceResponse, 0, len(preview.Sources)), Warnings: preview.Warnings}
	for _, source := range preview.Sources {
		response.Sources = append(response.Sources, PreviewSourceResponse{SourceConnectionID: source.SourceConnectionID, QuerySignature: source.QuerySignature, CompiledQuery: source.CompiledQuery, QueryMode: source.QueryMode, Languages: source.Languages, Regions: source.Regions, MaxQueryBytes: source.MaxQueryBytes, IncludedRuleIDs: source.IncludedRuleIDs, ExcludedRuleIDs: source.ExcludedRuleIDs, UnapprovedRuleIDs: source.UnapprovedRuleIDs, IncludedTermCount: source.IncludedTermCount, ExcludedTermCount: source.ExcludedTermCount, EstimatedRequests: source.EstimatedRequests})
		response.EstimatedRequests += source.EstimatedRequests
	}
	return response
}
