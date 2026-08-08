package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/domain"
)

type AgentAccessResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type CreateTokenRequest struct {
	Name         string   `json:"name" binding:"required,min=1,max=64" example:"Research agent"`
	Scopes       []string `json:"scopes" binding:"required,min=1,max=6,dive,required" enums:"monitors.read,events.read,contents.read,reports.read,search.run,alerts.write"`
	LifetimeDays int      `json:"lifetime_days" binding:"required,min=1,max=90" minimum:"1" maximum:"90" example:"30"`
}

type RevokeTokenRequest struct {
	ExpectedVersion int64 `json:"expected_version" binding:"required,gt=0" example:"1"`
}

type TokenResponse struct {
	ID          int64      `json:"id"`
	Version     int64      `json:"version"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      []string   `json:"scopes"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CreatedTokenResponse struct {
	TokenResponse
	Token string `json:"token"`
}

type EmptyResponse struct{}

func tokenResponse(token domain.Token) TokenResponse {
	return TokenResponse{ID: token.ID, Version: token.Version, Name: token.Name, TokenPrefix: token.TokenPrefix, Scopes: domain.ScopeStrings(token.Scopes), ExpiresAt: token.ExpiresAt, LastUsedAt: token.LastUsedAt, RevokedAt: token.RevokedAt, CreatedAt: token.CreatedAt}
}

func tokenResponses(tokens []domain.Token) []TokenResponse {
	result := make([]TokenResponse, 0, len(tokens))
	for _, token := range tokens {
		result = append(result, tokenResponse(token))
	}
	return result
}
