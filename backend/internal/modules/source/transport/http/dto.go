package http

import (
	"fmt"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/preset"
)

// SourceResult mirrors Result for Swagger only. Runtime handlers always use
// the platform helpers and therefore keep error data null.
type SourceResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type MetricCapabilityProfileResponse struct {
	ID                        int64      `json:"id"`
	Version                   int64      `json:"version"`
	SourceType                string     `json:"source_type"`
	ProfileVersion            string     `json:"profile_version"`
	SupportsViews             bool       `json:"supports_views"`
	SupportsLikes             bool       `json:"supports_likes"`
	SupportsComments          bool       `json:"supports_comments"`
	SupportsShares            bool       `json:"supports_shares"`
	IndependenceStrategy      string     `json:"independence_strategy"`
	NormalizationWindowHours  int        `json:"normalization_window_hours"`
	MaxSingleItemContribution float64    `json:"max_single_item_contribution"`
	Status                    string     `json:"status"`
	PublishedAt               *time.Time `json:"published_at,omitempty"`
	ArchivedAt                *time.Time `json:"archived_at,omitempty"`
}

type CreateMetricCapabilityProfileRequest struct {
	SourceType                string  `json:"source_type" binding:"required,oneof=rss hacker_news x bilibili weibo"`
	ProfileVersion            string  `json:"profile_version" binding:"required,max=64"`
	SupportsViews             bool    `json:"supports_views"`
	SupportsLikes             bool    `json:"supports_likes"`
	SupportsComments          bool    `json:"supports_comments"`
	SupportsShares            bool    `json:"supports_shares"`
	IndependenceStrategy      string  `json:"independence_strategy" binding:"required,oneof=source_connection author"`
	NormalizationWindowHours  int     `json:"normalization_window_hours" binding:"required,gte=1,lte=720"`
	MaxSingleItemContribution float64 `json:"max_single_item_contribution" binding:"required,gt=0,lte=100"`
}

type MetricCapabilityLifecycleRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required,gt=0"`
	ReasonCode      string `json:"reason_code" binding:"required,max=64"`
}

// CollectionResult mirrors Result for collection-control Swagger declarations.
// Runtime responses still use the shared transport helpers.
type CollectionResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type CollectionRunTargetResponse struct {
	ID             int64  `json:"id"`
	Status         string `json:"status"`
	CandidateCount int64  `json:"candidate_count"`
	AcceptedCount  int64  `json:"accepted_count"`
	RejectedCount  int64  `json:"rejected_count"`
	ErrorCode      string `json:"error_code,omitempty"`
}

type CollectionRunResponse struct {
	ID             int64                         `json:"id"`
	Status         string                        `json:"status"`
	CandidateCount int64                         `json:"candidate_count"`
	AcceptedCount  int64                         `json:"accepted_count"`
	RejectedCount  int64                         `json:"rejected_count"`
	ErrorCode      string                        `json:"error_code,omitempty"`
	StartedAt      *time.Time                    `json:"started_at,omitempty"`
	FinishedAt     *time.Time                    `json:"finished_at,omitempty"`
	Targets        []CollectionRunTargetResponse `json:"targets"`
}

type CollectionRunPageResponse struct {
	Items      []CollectionRunResponse `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type ManualCollectionResponse struct {
	Requested     int       `json:"requested"`
	Created       int       `json:"created"`
	Reused        int       `json:"reused"`
	CooldownUntil time.Time `json:"cooldown_until"`
}

type MonitorScanSourceResponse struct {
	RunID              int64      `json:"run_id"`
	SourceConnectionID int64      `json:"source_connection_id"`
	SourceName         string     `json:"source_name"`
	SourceType         string     `json:"source_type"`
	TriggerType        string     `json:"trigger_type" enums:"schedule,manual,retry,reconcile"`
	Status             string     `json:"status" enums:"queued,running,succeeded,failed,cancelled"`
	CandidateCount     int64      `json:"candidate_count"`
	AcceptedCount      int64      `json:"accepted_count"`
	RejectedCount      int64      `json:"rejected_count"`
	ErrorCode          string     `json:"error_code,omitempty"`
	ScheduledAt        time.Time  `json:"scheduled_at"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
}

type MonitorScanPageResponse struct {
	Items []MonitorScanResponse `json:"items"`
}

type MonitorScanResponse struct {
	ID             string                      `json:"id"`
	MonitorID      int64                       `json:"monitor_id"`
	TriggerType    string                      `json:"trigger_type" enums:"schedule,manual,retry,reconcile"`
	Status         string                      `json:"status" enums:"queued,running,succeeded,partial,failed"`
	RunOutcome     string                      `json:"run_outcome,omitempty" enums:"success,partial_success,failed"`
	CandidateCount int64                       `json:"candidate_count"`
	AcceptedCount  int64                       `json:"accepted_count"`
	RejectedCount  int64                       `json:"rejected_count"`
	ScheduledAt    time.Time                   `json:"scheduled_at"`
	StartedAt      *time.Time                  `json:"started_at,omitempty"`
	FinishedAt     *time.Time                  `json:"finished_at,omitempty"`
	Sources        []MonitorScanSourceResponse `json:"sources"`
}

type SourceHealthResponse struct {
	Healthy   bool      `json:"healthy"`
	CheckedAt time.Time `json:"checked_at"`
	ErrorCode string    `json:"error_code,omitempty"`
}

// SourceConfigRequest is a fixed whitelist. It deliberately has no generic
// map and no credential-shaped field, so management DTOs cannot become a
// vehicle for arbitrary secret JSON.
type SourceConfigRequest struct {
	// Deprecated: retained for legacy source configuration compatibility. This
	// value never authorizes v2 raw evidence or document body persistence.
	AllowBodyStorage                 *bool     `json:"allow_body_storage,omitempty"`
	RequiresAttribution              *bool     `json:"requires_attribution,omitempty"`
	RequiresDeletionSync             *bool     `json:"requires_deletion_sync,omitempty"`
	ContentRetentionDays             *int      `json:"content_retention_days,omitempty"`
	MetricsRetentionDays             *int      `json:"metrics_retention_days,omitempty"`
	AllowedLanguages                 *[]string `json:"allowed_languages,omitempty"`
	AllowedRegions                   *[]string `json:"allowed_regions,omitempty"`
	RateLimitPerMinute               *int      `json:"rate_limit_per_minute,omitempty"`
	RequestTimeoutSeconds            *int      `json:"request_timeout_seconds,omitempty"`
	MaxPagesPerRun                   *int      `json:"max_pages_per_run,omitempty"`
	GroundingDataBoundaryApproved    *bool     `json:"grounding_data_boundary_approved,omitempty"`
	BilibiliOpenID                   *string   `json:"bilibili_open_id,omitempty"`
	GoogleLocation                   *string   `json:"google_location,omitempty"`
	GoogleServingConfig              *string   `json:"google_serving_config,omitempty"`
	HackerNewsMode                   *string   `json:"hacker_news_mode,omitempty" binding:"omitempty,oneof=new top best"`
	XMetricRefreshEnabled            *bool     `json:"x_metric_refresh_enabled,omitempty"`
	XMetricRefreshIntervalMinutes    *int      `json:"x_metric_refresh_interval_minutes,omitempty" binding:"omitempty,min=15,max=1440"`
	XMetricRefreshObservationHours   *int      `json:"x_metric_refresh_observation_hours,omitempty" binding:"omitempty,min=1,max=168"`
	XMetricRefreshMaxPostsPerRun     *int      `json:"x_metric_refresh_max_posts_per_run,omitempty" binding:"omitempty,min=1,max=100"`
	XMetricRefreshDailyRequestBudget *int      `json:"x_metric_refresh_daily_request_budget,omitempty" binding:"omitempty,min=1,max=1440"`
}

type CreateSourceRequest struct {
	PresetID       string                     `json:"preset_id,omitempty" binding:"omitempty,max=64"`
	PresetValues   []SourcePresetValueRequest `json:"preset_values,omitempty" binding:"omitempty,max=10"`
	SourceType     string                     `json:"source_type,omitempty" binding:"omitempty,oneof=rss hacker_news x bing_grounding bilibili weibo google_agent_search"`
	Name           string                     `json:"name" binding:"required"`
	Endpoint       string                     `json:"endpoint,omitempty"`
	AuthType       string                     `json:"auth_type,omitempty" binding:"omitempty,oneof=none api_key oauth2 bearer"`
	CredentialRef  string                     `json:"credential_ref"`
	Credential     *string                    `json:"credential,omitempty"`
	Config         SourceConfigRequest        `json:"config"`
	Enabled        *bool                      `json:"enabled,omitempty"`
	TermsPolicyURL string                     `json:"terms_policy_url"`
}

type SourcePresetValueRequest struct {
	Key   string `json:"key" binding:"required,max=64"`
	Value string `json:"value" binding:"required,max=2048"`
}

type SourcePresetInputResponse struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
	Required    bool   `json:"required"`
	MaxLength   int    `json:"max_length"`
}

type SourcePresetResponse struct {
	ID                 string                      `json:"id"`
	Label              string                      `json:"label"`
	Description        string                      `json:"description"`
	SourceType         string                      `json:"source_type"`
	AuthLabel          string                      `json:"auth_label"`
	Cost               string                      `json:"cost" enums:"free,paid,credentialed"`
	CredentialRequired bool                        `json:"credential_required"`
	Inputs             []SourcePresetInputResponse `json:"inputs"`
}

type SourcePresetPageResponse struct {
	Items []SourcePresetResponse `json:"items"`
}

type UpdateSourceRequest struct {
	ExpectedSourceVersion int64                `json:"expected_source_version" binding:"required,gt=0"`
	SourceType            *string              `json:"source_type,omitempty" binding:"omitempty,oneof=rss hacker_news x bing_grounding bilibili weibo google_agent_search"`
	Name                  *string              `json:"name,omitempty"`
	Endpoint              *string              `json:"endpoint,omitempty"`
	AuthType              *string              `json:"auth_type,omitempty" binding:"omitempty,oneof=none api_key oauth2 bearer"`
	CredentialRef         *string              `json:"credential_ref,omitempty"`
	Credential            *string              `json:"credential,omitempty"`
	Config                *SourceConfigRequest `json:"config,omitempty"`
	TermsPolicyURL        *string              `json:"terms_policy_url,omitempty"`
}

type SourceLifecycleRequest struct {
	ExpectedSourceVersion int64 `json:"expected_source_version" binding:"required,gt=0"`
}

type SourceResponse struct {
	ID                   int64  `json:"id"`
	Version              int64  `json:"version"`
	Name                 string `json:"name"`
	SourceType           string `json:"source_type"`
	Enabled              bool   `json:"enabled"`
	HealthStatus         string `json:"health_status"`
	TermsPolicyURL       string `json:"terms_policy_url"`
	CredentialConfigured bool   `json:"credential_configured"`
	Deleted              bool   `json:"deleted"`
}

// ManagementSourceResponse intentionally exposes only endpoint and the fixed
// allowlisted non-secret config in addition to SourceResponse. CredentialRef
// and diagnostics are absent for every role, including admin.
type ManagementSourceResponse struct {
	SourceResponse
	Endpoint string          `json:"endpoint"`
	Config   SourceConfigDTO `json:"config"`
}

// SourceReadResponse documents the role-dependent GET/List union. Every
// authenticated caller receives the public SourceResponse fields; endpoint and
// allowlisted config are present only for administrators. Neither branch ever
// contains credential references or health diagnostics.
type SourceReadResponse struct {
	SourceResponse
	Endpoint *string          `json:"endpoint,omitempty"`
	Config   *SourceConfigDTO `json:"config,omitempty"`
}

type SourceConfigDTO struct {
	// Deprecated: informational legacy configuration only; not a rights grant.
	AllowBodyStorage                 bool     `json:"allow_body_storage"`
	RequiresAttribution              bool     `json:"requires_attribution"`
	RequiresDeletionSync             bool     `json:"requires_deletion_sync"`
	ContentRetentionDays             int      `json:"content_retention_days"`
	MetricsRetentionDays             int      `json:"metrics_retention_days"`
	AllowedLanguages                 []string `json:"allowed_languages"`
	AllowedRegions                   []string `json:"allowed_regions"`
	RateLimitPerMinute               int      `json:"rate_limit_per_minute"`
	RequestTimeoutSeconds            int      `json:"request_timeout_seconds"`
	MaxPagesPerRun                   int      `json:"max_pages_per_run"`
	GroundingDataBoundaryApproved    bool     `json:"grounding_data_boundary_approved"`
	BilibiliOpenID                   string   `json:"bilibili_open_id"`
	GoogleLocation                   string   `json:"google_location"`
	GoogleServingConfig              string   `json:"google_serving_config"`
	HackerNewsMode                   string   `json:"hacker_news_mode"`
	XMetricRefreshEnabled            bool     `json:"x_metric_refresh_enabled"`
	XMetricRefreshIntervalMinutes    int      `json:"x_metric_refresh_interval_minutes"`
	XMetricRefreshObservationHours   int      `json:"x_metric_refresh_observation_hours"`
	XMetricRefreshMaxPostsPerRun     int      `json:"x_metric_refresh_max_posts_per_run"`
	XMetricRefreshDailyRequestBudget int      `json:"x_metric_refresh_daily_request_budget"`
}

type SourcePageResponse struct {
	Items      []SourceResponse `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

type ManagementSourcePageResponse struct {
	Items      []ManagementSourceResponse `json:"items"`
	NextCursor string                     `json:"next_cursor"`
}

type SourceReadPageResponse struct {
	Items      []SourceReadResponse `json:"items"`
	NextCursor string               `json:"next_cursor"`
}

func sourceConfig(request SourceConfigRequest) (domain.SourceConfig, error) {
	values := map[string]any{}
	if request.AllowBodyStorage != nil {
		values["allow_body_storage"] = *request.AllowBodyStorage
	}
	if request.RequiresAttribution != nil {
		values["requires_attribution"] = *request.RequiresAttribution
	}
	if request.RequiresDeletionSync != nil {
		values["requires_deletion_sync"] = *request.RequiresDeletionSync
	}
	if request.ContentRetentionDays != nil {
		values["content_retention_days"] = *request.ContentRetentionDays
	}
	if request.MetricsRetentionDays != nil {
		values["metrics_retention_days"] = *request.MetricsRetentionDays
	}
	if request.AllowedLanguages != nil {
		values["allowed_languages"] = *request.AllowedLanguages
	}
	if request.AllowedRegions != nil {
		values["allowed_regions"] = *request.AllowedRegions
	}
	if request.RateLimitPerMinute != nil {
		values["rate_limit_per_minute"] = *request.RateLimitPerMinute
	}
	if request.RequestTimeoutSeconds != nil {
		values["request_timeout_seconds"] = *request.RequestTimeoutSeconds
	}
	if request.MaxPagesPerRun != nil {
		values["max_pages_per_run"] = *request.MaxPagesPerRun
	}
	if request.GroundingDataBoundaryApproved != nil {
		values["grounding_data_boundary_approved"] = *request.GroundingDataBoundaryApproved
	}
	if request.BilibiliOpenID != nil {
		values["bilibili_open_id"] = *request.BilibiliOpenID
	}
	if request.GoogleLocation != nil {
		values["google_location"] = *request.GoogleLocation
	}
	if request.GoogleServingConfig != nil {
		values["google_serving_config"] = *request.GoogleServingConfig
	}
	if request.HackerNewsMode != nil {
		values["hacker_news_mode"] = *request.HackerNewsMode
	}
	if request.XMetricRefreshEnabled != nil {
		values["x_metric_refresh_enabled"] = *request.XMetricRefreshEnabled
	}
	if request.XMetricRefreshIntervalMinutes != nil {
		values["x_metric_refresh_interval_minutes"] = *request.XMetricRefreshIntervalMinutes
	}
	if request.XMetricRefreshObservationHours != nil {
		values["x_metric_refresh_observation_hours"] = *request.XMetricRefreshObservationHours
	}
	if request.XMetricRefreshMaxPostsPerRun != nil {
		values["x_metric_refresh_max_posts_per_run"] = *request.XMetricRefreshMaxPostsPerRun
	}
	if request.XMetricRefreshDailyRequestBudget != nil {
		values["x_metric_refresh_daily_request_budget"] = *request.XMetricRefreshDailyRequestBudget
	}
	config, err := domain.NormalizeSourceConfig(values)
	if err != nil {
		return domain.SourceConfig{}, fmt.Errorf("normalize source config: %w", err)
	}
	return config, nil
}

func sourceCreateInput(request CreateSourceRequest) (domain.SourceConnection, error) {
	config, err := sourceConfig(request.Config)
	if err != nil {
		return domain.SourceConnection{}, err
	}
	if request.PresetID != "" {
		if request.SourceType != "" || request.Endpoint != "" || request.AuthType != "" || request.Enabled != nil || request.TermsPolicyURL != "" {
			return domain.SourceConnection{}, fmt.Errorf("preset creation cannot override server-managed connection fields")
		}
		values := make(map[string]string, len(request.PresetValues))
		for _, item := range request.PresetValues {
			if item.Key == "" || len(item.Key) > 64 || len([]rune(item.Value)) > 2048 {
				return domain.SourceConnection{}, fmt.Errorf("invalid preset value")
			}
			if _, exists := values[item.Key]; exists {
				return domain.SourceConnection{}, fmt.Errorf("duplicate preset value %q", item.Key)
			}
			values[item.Key] = item.Value
		}
		return preset.Resolve(request.PresetID, request.Name, values, config)
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	return domain.SourceConnection{SourceType: domain.SourceType(request.SourceType), Name: request.Name, Endpoint: request.Endpoint, AuthType: domain.AuthType(request.AuthType), CredentialRef: request.CredentialRef, Config: config, Enabled: enabled, TermsPolicyURL: request.TermsPolicyURL}, nil
}

func sourcePresetPageResponse() SourcePresetPageResponse {
	definitions := preset.Catalog()
	response := SourcePresetPageResponse{Items: make([]SourcePresetResponse, 0, len(definitions))}
	for _, definition := range definitions {
		item := SourcePresetResponse{ID: definition.ID, Label: definition.Label, Description: definition.Description, SourceType: string(definition.SourceType), AuthLabel: definition.AuthLabel, Cost: string(definition.Cost), CredentialRequired: definition.CredentialRequired, Inputs: make([]SourcePresetInputResponse, 0, len(definition.Inputs))}
		for _, input := range definition.Inputs {
			item.Inputs = append(item.Inputs, SourcePresetInputResponse{Key: input.Key, Label: input.Label, Placeholder: input.Placeholder, Required: input.Required, MaxLength: input.MaxLength})
		}
		response.Items = append(response.Items, item)
	}
	return response
}

func sourceUpdateInput(request UpdateSourceRequest) (sourceapplication.UpdateInput, error) {
	input := sourceapplication.UpdateInput{ExpectedVersion: request.ExpectedSourceVersion, Name: request.Name, Endpoint: request.Endpoint, CredentialRef: request.CredentialRef, Credential: request.Credential, TermsPolicyURL: request.TermsPolicyURL}
	if request.SourceType != nil {
		value := domain.SourceType(*request.SourceType)
		input.SourceType = &value
	}
	if request.AuthType != nil {
		value := domain.AuthType(*request.AuthType)
		input.AuthType = &value
	}
	if request.Config != nil {
		value, err := sourceConfig(*request.Config)
		if err != nil {
			return sourceapplication.UpdateInput{}, err
		}
		input.Config = &value
	}
	return input, nil
}

func sourceResponse(source domain.PublicSourceConnection) SourceResponse {
	return SourceResponse{ID: source.ID, Version: source.Version, Name: source.Name, SourceType: string(source.SourceType), Enabled: source.Enabled, HealthStatus: string(source.HealthStatus), TermsPolicyURL: source.TermsPolicyURL, CredentialConfigured: source.CredentialConfigured, Deleted: source.Deleted}
}
func managementResponse(source domain.ManagementSourceConnection) ManagementSourceResponse {
	return ManagementSourceResponse{SourceResponse: sourceResponse(source.PublicSourceConnection), Endpoint: source.Endpoint, Config: configResponse(source.Config)}
}
func sourceReadResponse(source domain.PublicSourceConnection) SourceReadResponse {
	return SourceReadResponse{SourceResponse: sourceResponse(source)}
}
func managementReadResponse(source domain.ManagementSourceConnection) SourceReadResponse {
	endpoint, config := source.Endpoint, configResponse(source.Config)
	return SourceReadResponse{SourceResponse: sourceResponse(source.PublicSourceConnection), Endpoint: &endpoint, Config: &config}
}
func configResponse(config domain.SourceConfig) SourceConfigDTO {
	return SourceConfigDTO{AllowBodyStorage: config.AllowBodyStorage, RequiresAttribution: config.RequiresAttribution, RequiresDeletionSync: config.RequiresDeletionSync, ContentRetentionDays: config.ContentRetentionDays, MetricsRetentionDays: config.MetricsRetentionDays, AllowedLanguages: config.AllowedLanguages, AllowedRegions: config.AllowedRegions, RateLimitPerMinute: config.RateLimitPerMinute, RequestTimeoutSeconds: config.RequestTimeoutSeconds, MaxPagesPerRun: config.MaxPagesPerRun, GroundingDataBoundaryApproved: config.GroundingDataBoundaryApproved, BilibiliOpenID: config.BilibiliOpenID, GoogleLocation: config.GoogleLocation, GoogleServingConfig: config.GoogleServingConfig, HackerNewsMode: string(config.HackerNewsMode), XMetricRefreshEnabled: config.XMetricRefreshEnabled, XMetricRefreshIntervalMinutes: config.XMetricRefreshIntervalMinutes, XMetricRefreshObservationHours: config.XMetricRefreshObservationHours, XMetricRefreshMaxPostsPerRun: config.XMetricRefreshMaxPostsPerRun, XMetricRefreshDailyRequestBudget: config.XMetricRefreshDailyRequestBudget}
}

func metricCapabilityProfileResponse(profile domain.MetricCapabilityProfile) MetricCapabilityProfileResponse {
	return MetricCapabilityProfileResponse{
		ID: profile.ID, Version: profile.Version, SourceType: string(profile.SourceType), ProfileVersion: profile.ProfileVersion,
		SupportsViews: profile.SupportsViews, SupportsLikes: profile.SupportsLikes, SupportsComments: profile.SupportsComments,
		SupportsShares: profile.SupportsShares, IndependenceStrategy: string(profile.IndependenceStrategy),
		NormalizationWindowHours:  profile.NormalizationWindowHours,
		MaxSingleItemContribution: profile.MaxSingleItemContribution, Status: string(profile.Status),
		PublishedAt: profile.PublishedAt, ArchivedAt: profile.ArchivedAt,
	}
}

func collectionRunPageResponse(page domain.CollectionRunPage) CollectionRunPageResponse {
	response := CollectionRunPageResponse{Items: make([]CollectionRunResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		response.Items = append(response.Items, collectionRunResponse(item))
	}
	return response
}

func manualCollectionResponse(summary domain.ManualCollectionSummary) ManualCollectionResponse {
	return ManualCollectionResponse{
		Requested: summary.Requested, Created: summary.Created, Reused: summary.Reused,
		CooldownUntil: summary.CooldownUntil.UTC(),
	}
}

func monitorScanPageResponse(items []domain.MonitorScan) MonitorScanPageResponse {
	response := MonitorScanPageResponse{Items: make([]MonitorScanResponse, 0, len(items))}
	for _, item := range items {
		scan := MonitorScanResponse{
			ID: item.ID, MonitorID: item.MonitorID, TriggerType: string(item.TriggerType), Status: string(item.Status),
			RunOutcome:     string(item.RunOutcome),
			CandidateCount: item.CandidateCount, AcceptedCount: item.AcceptedCount, RejectedCount: item.RejectedCount,
			ScheduledAt: item.ScheduledAt.UTC(), StartedAt: item.StartedAt, FinishedAt: item.FinishedAt,
			Sources: make([]MonitorScanSourceResponse, 0, len(item.Sources)),
		}
		for _, source := range item.Sources {
			scan.Sources = append(scan.Sources, MonitorScanSourceResponse{
				RunID: source.RunID, SourceConnectionID: source.SourceConnectionID,
				SourceName: source.SourceName, SourceType: source.SourceType,
				TriggerType: string(source.TriggerType), Status: string(source.Status),
				CandidateCount: source.CandidateCount, AcceptedCount: source.AcceptedCount,
				RejectedCount: source.RejectedCount, ErrorCode: source.ErrorCode,
				ScheduledAt: source.ScheduledAt.UTC(), StartedAt: source.StartedAt, FinishedAt: source.FinishedAt,
			})
		}
		response.Items = append(response.Items, scan)
	}
	return response
}

func collectionRunResponse(summary domain.CollectionRunSummary) CollectionRunResponse {
	response := CollectionRunResponse{
		ID: summary.ID, Status: string(summary.Status), CandidateCount: summary.CandidateCount,
		AcceptedCount: summary.AcceptedCount, RejectedCount: summary.RejectedCount, ErrorCode: summary.ErrorCode,
		StartedAt: summary.StartedAt, FinishedAt: summary.FinishedAt,
		Targets: make([]CollectionRunTargetResponse, 0, len(summary.Targets)),
	}
	for _, target := range summary.Targets {
		response.Targets = append(response.Targets, CollectionRunTargetResponse{
			ID: target.ID, Status: string(target.Status), CandidateCount: target.CandidateCount,
			AcceptedCount: target.AcceptedCount, RejectedCount: target.RejectedCount, ErrorCode: target.ErrorCode,
		})
	}
	return response
}

func sourceHealthResponse(health domain.SourceHealth) SourceHealthResponse {
	return SourceHealthResponse{Healthy: health.Healthy, CheckedAt: health.CheckedAt, ErrorCode: health.ErrorCode}
}
