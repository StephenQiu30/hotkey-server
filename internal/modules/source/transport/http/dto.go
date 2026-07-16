package http

import (
	"fmt"

	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

// SourceResult mirrors Result for Swagger only. Runtime handlers always use
// the platform helpers and therefore keep error data null.
type SourceResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

// SourceConfigRequest is a fixed whitelist. It deliberately has no generic
// map and no credential-shaped field, so management DTOs cannot become a
// vehicle for arbitrary secret JSON.
type SourceConfigRequest struct {
	AllowBodyStorage      *bool     `json:"allow_body_storage,omitempty"`
	RequiresAttribution   *bool     `json:"requires_attribution,omitempty"`
	RequiresDeletionSync  *bool     `json:"requires_deletion_sync,omitempty"`
	ContentRetentionDays  *int      `json:"content_retention_days,omitempty"`
	MetricsRetentionDays  *int      `json:"metrics_retention_days,omitempty"`
	AllowedLanguages      *[]string `json:"allowed_languages,omitempty"`
	AllowedRegions        *[]string `json:"allowed_regions,omitempty"`
	RateLimitPerMinute    *int      `json:"rate_limit_per_minute,omitempty"`
	RequestTimeoutSeconds *int      `json:"request_timeout_seconds,omitempty"`
	MaxPagesPerRun        *int      `json:"max_pages_per_run,omitempty"`
}

type CreateSourceRequest struct {
	SourceType     string              `json:"source_type" binding:"required,oneof=rss hacker_news"`
	Name           string              `json:"name" binding:"required"`
	Endpoint       string              `json:"endpoint" binding:"required"`
	AuthType       string              `json:"auth_type" binding:"required,oneof=none api_key oauth2 bearer"`
	CredentialRef  string              `json:"credential_ref"`
	Config         SourceConfigRequest `json:"config"`
	Enabled        *bool               `json:"enabled,omitempty"`
	TermsPolicyURL string              `json:"terms_policy_url"`
}

type UpdateSourceRequest struct {
	ExpectedSourceVersion int64                `json:"expected_source_version" binding:"required,gt=0"`
	SourceType            *string              `json:"source_type,omitempty" binding:"omitempty,oneof=rss hacker_news"`
	Name                  *string              `json:"name,omitempty"`
	Endpoint              *string              `json:"endpoint,omitempty"`
	AuthType              *string              `json:"auth_type,omitempty" binding:"omitempty,oneof=none api_key oauth2 bearer"`
	CredentialRef         *string              `json:"credential_ref,omitempty"`
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

type SourceConfigDTO struct {
	AllowBodyStorage      bool     `json:"allow_body_storage"`
	RequiresAttribution   bool     `json:"requires_attribution"`
	RequiresDeletionSync  bool     `json:"requires_deletion_sync"`
	ContentRetentionDays  int      `json:"content_retention_days"`
	MetricsRetentionDays  int      `json:"metrics_retention_days"`
	AllowedLanguages      []string `json:"allowed_languages"`
	AllowedRegions        []string `json:"allowed_regions"`
	RateLimitPerMinute    int      `json:"rate_limit_per_minute"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
	MaxPagesPerRun        int      `json:"max_pages_per_run"`
}

type SourcePageResponse struct {
	Items      []SourceResponse `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

type ManagementSourcePageResponse struct {
	Items      []ManagementSourceResponse `json:"items"`
	NextCursor string                     `json:"next_cursor"`
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
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	return domain.SourceConnection{SourceType: domain.SourceType(request.SourceType), Name: request.Name, Endpoint: request.Endpoint, AuthType: domain.AuthType(request.AuthType), CredentialRef: request.CredentialRef, Config: config, Enabled: enabled, TermsPolicyURL: request.TermsPolicyURL}, nil
}

func sourceUpdateInput(request UpdateSourceRequest) (sourceapplication.UpdateInput, error) {
	input := sourceapplication.UpdateInput{ExpectedVersion: request.ExpectedSourceVersion, Name: request.Name, Endpoint: request.Endpoint, CredentialRef: request.CredentialRef, TermsPolicyURL: request.TermsPolicyURL}
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
func configResponse(config domain.SourceConfig) SourceConfigDTO {
	return SourceConfigDTO{AllowBodyStorage: config.AllowBodyStorage, RequiresAttribution: config.RequiresAttribution, RequiresDeletionSync: config.RequiresDeletionSync, ContentRetentionDays: config.ContentRetentionDays, MetricsRetentionDays: config.MetricsRetentionDays, AllowedLanguages: config.AllowedLanguages, AllowedRegions: config.AllowedRegions, RateLimitPerMinute: config.RateLimitPerMinute, RequestTimeoutSeconds: config.RequestTimeoutSeconds, MaxPagesPerRun: config.MaxPagesPerRun}
}
