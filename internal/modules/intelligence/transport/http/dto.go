// Package http exposes the administrator-only AI model profile control plane.
// Its DTOs are explicit allowlists: credential references are write-only and
// never appear in a response or generated OpenAPI schema.
package http

import (
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

// ModelProfileResult mirrors the shared Result envelope for Swagger only.
// Runtime handlers always use the platform HTTP result helpers.
type ModelProfileResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

// CreateModelProfileRequest is the only write path for credential_ref. Its
// value is validated as a reference, not resolved by this transport.
type CreateModelProfileRequest struct {
	Name                string  `json:"name" example:"embedding-primary"`
	TaskType            string  `json:"task_type" enums:"embedding,term_expansion,relevance_review,event_summary,entity_claim_extraction"`
	Provider            string  `json:"provider" enums:"openai,onnx"`
	ModelName           string  `json:"model_name" example:"text-embedding-3-large"`
	ModelVersion        string  `json:"model_version" example:"2026-07"`
	CredentialRef       *string `json:"credential_ref"`
	EmbeddingDimensions *int    `json:"embedding_dimensions,omitempty" example:"1024"`
	TimeoutSeconds      int     `json:"timeout_seconds" example:"30"`
	MaxAttempts         int     `json:"max_attempts" example:"2"`
	MaxCost             string  `json:"max_cost" example:"0.1000"`
	DailyBudget         *string `json:"daily_budget,omitempty" example:"10.0000"`
	FallbackPriority    int     `json:"fallback_priority" example:"100"`
	Enabled             bool    `json:"enabled"`
}

// UpdateModelProfileRequest contains only mutable controls. Immutable profile
// fields are intentionally absent from this type and its OpenAPI schema; the
// handler still recognizes their JSON names solely to return stable 70000.
type UpdateModelProfileRequest struct {
	Version          int64   `json:"version" example:"1"`
	TimeoutSeconds   *int    `json:"timeout_seconds,omitempty" example:"30"`
	MaxAttempts      *int    `json:"max_attempts,omitempty" example:"2"`
	MaxCost          *string `json:"max_cost,omitempty" example:"0.1000"`
	DailyBudget      *string `json:"daily_budget,omitempty" extensions:"x-nullable"`
	FallbackPriority *int    `json:"fallback_priority,omitempty" example:"100"`
	Enabled          *bool   `json:"enabled,omitempty"`
}

type ModelProfileVersionRequest struct {
	Version int64 `json:"version" example:"1"`
}

// ModelProfileResponse is deliberately credential-free. It also omits
// provider endpoints, arbitrary parameters, prompts and raw provider output.
type ModelProfileResponse struct {
	ID                  int64     `json:"id" example:"7"`
	Version             int64     `json:"version" example:"1"`
	Name                string    `json:"name" example:"embedding-primary"`
	TaskType            string    `json:"task_type" example:"embedding"`
	Provider            string    `json:"provider" example:"openai"`
	ModelName           string    `json:"model_name" example:"text-embedding-3-large"`
	ModelVersion        string    `json:"model_version" example:"2026-07"`
	EmbeddingDimensions *int      `json:"embedding_dimensions,omitempty" extensions:"x-nullable"`
	TimeoutSeconds      int       `json:"timeout_seconds" example:"30"`
	MaxAttempts         int       `json:"max_attempts" example:"2"`
	MaxCost             string    `json:"max_cost" example:"0.1000"`
	DailyBudget         *string   `json:"daily_budget,omitempty" extensions:"x-nullable"`
	FallbackPriority    int       `json:"fallback_priority" example:"100"`
	Enabled             bool      `json:"enabled"`
	Deleted             bool      `json:"deleted"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ModelProfileListResponse struct {
	Items []ModelProfileResponse `json:"items"`
}

func createModelProfile(request CreateModelProfileRequest) intelligencedomain.ModelProfile {
	return intelligencedomain.ModelProfile{
		Name: request.Name, TaskType: intelligencedomain.TaskType(request.TaskType), Provider: intelligencedomain.ProviderName(request.Provider),
		ModelName: request.ModelName, ModelVersion: request.ModelVersion, CredentialRef: request.CredentialRef,
		EmbeddingDimensions: request.EmbeddingDimensions, TimeoutSeconds: request.TimeoutSeconds, MaxAttempts: request.MaxAttempts,
		MaxCost: request.MaxCost, DailyBudget: request.DailyBudget, FallbackPriority: request.FallbackPriority, Enabled: request.Enabled,
	}
}

func modelProfileResponse(profile intelligencedomain.ModelProfile) ModelProfileResponse {
	return ModelProfileResponse{
		ID: profile.ID, Version: profile.Version, Name: profile.Name, TaskType: string(profile.TaskType), Provider: string(profile.Provider),
		ModelName: profile.ModelName, ModelVersion: profile.ModelVersion, EmbeddingDimensions: profile.EmbeddingDimensions,
		TimeoutSeconds: profile.TimeoutSeconds, MaxAttempts: profile.MaxAttempts, MaxCost: profile.MaxCost, DailyBudget: profile.DailyBudget,
		FallbackPriority: profile.FallbackPriority, Enabled: profile.Enabled, Deleted: profile.Deleted,
		CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt,
	}
}

func modelProfileListResponse(profiles []intelligencedomain.ModelProfile) ModelProfileListResponse {
	items := make([]ModelProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, modelProfileResponse(profile))
	}
	return ModelProfileListResponse{Items: items}
}
