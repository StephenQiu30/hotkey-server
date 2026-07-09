package controller

import (
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// Request types

type RegisterRequest struct {
	Email       string `json:"email" example:"user@example.com"`
	Password    string `json:"password" example:"Passw0rd!"`
	DisplayName string `json:"display_name" example:"Stephen"`
}

type LoginRequest struct {
	Email    string `json:"email" example:"user@example.com"`
	Password string `json:"password" example:"Passw0rd!"`
}

type CreateMonitorRequest struct {
	Name                string `json:"name" example:"AI monitor"`
	QueryText           string `json:"query_text" example:"openai OR gpt"`
	Language            string `json:"language,omitempty" example:"en"`
	Region              string `json:"region,omitempty" example:"US"`
	PollIntervalMinutes int    `json:"poll_interval_minutes" example:"15"`
	AlertEnabled        bool   `json:"alert_enabled" example:"true"`
}

type UpdateMonitorRequest struct {
	Name                *string `json:"name,omitempty" example:"AI monitor"`
	QueryText           *string `json:"query_text,omitempty" example:"openai OR gpt"`
	Language            *string `json:"language,omitempty" example:"en"`
	Region              *string `json:"region,omitempty" example:"US"`
	PollIntervalMinutes *int    `json:"poll_interval_minutes,omitempty" example:"15"`
	AlertEnabled        *bool   `json:"alert_enabled,omitempty" example:"true"`
	Status              *string `json:"status,omitempty" example:"active"`
}

// Response types for swagger documentation

type HealthResponse struct {
	Data      vo.HealthBody `json:"data"`
	RequestID string        `json:"request_id,omitempty"`
}

type UserResponse struct {
	Data      vo.UserData `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}

type LoginResponse struct {
	Data      vo.LoginData `json:"data"`
	RequestID string       `json:"request_id,omitempty"`
}

type MonitorResponse struct {
	Data      vo.MonitorData `json:"data"`
	RequestID string         `json:"request_id,omitempty"`
}

type MonitorListResponse struct {
	Data      []vo.MonitorData `json:"data"`
	RequestID string           `json:"request_id,omitempty"`
}

type PostListResponse struct {
	Data      []content.PostSummary `json:"data"`
	RequestID string                `json:"request_id,omitempty"`
}

type TopicListResponse struct {
	Data      []service.TopicSummary `json:"data"`
	RequestID string                 `json:"request_id,omitempty"`
}

type TrendListResponse struct {
	Data      []service.TrendPoint `json:"data"`
	RequestID string               `json:"request_id,omitempty"`
}

type NotificationListResponse struct {
	Data      []vo.NotificationData `json:"data"`
	RequestID string                `json:"request_id,omitempty"`
}

type MarkNotificationReadResponse struct {
	Data      vo.MarkNotificationReadData `json:"data"`
	RequestID string                      `json:"request_id,omitempty"`
}
