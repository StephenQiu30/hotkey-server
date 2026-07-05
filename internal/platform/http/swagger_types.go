package http

import (
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
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

// Response types

type HealthResponse struct {
	Data      HealthBody `json:"data"`
	RequestID string     `json:"request_id,omitempty"`
}

type UserResponse struct {
	Data      UserData `json:"data"`
	RequestID string   `json:"request_id,omitempty"`
}

type LoginResponse struct {
	Data      LoginData `json:"data"`
	RequestID string    `json:"request_id,omitempty"`
}

type MonitorResponse struct {
	Data      MonitorData `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}

type MonitorListResponse struct {
	Data      []MonitorData `json:"data"`
	RequestID string        `json:"request_id,omitempty"`
}

type PostListResponse struct {
	Data      []content.PostSummary `json:"data"`
	RequestID string                `json:"request_id,omitempty"`
}

type TopicListResponse struct {
	Data      []topic.TopicSummary `json:"data"`
	RequestID string               `json:"request_id,omitempty"`
}

type TrendListResponse struct {
	Data      []trend.TrendPoint `json:"data"`
	RequestID string             `json:"request_id,omitempty"`
}

type NotificationListResponse struct {
	Data      []NotificationData `json:"data"`
	RequestID string             `json:"request_id,omitempty"`
}

type MarkNotificationReadData struct {
	Read bool `json:"read" example:"true"`
}

type MarkNotificationReadResponse struct {
	Data      MarkNotificationReadData `json:"data"`
	RequestID string                   `json:"request_id,omitempty"`
}
