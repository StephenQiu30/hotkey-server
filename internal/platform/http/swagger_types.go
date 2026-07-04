package http

import (
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

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

type HealthEnvelope struct {
	Data      HealthBody `json:"data"`
	RequestID string     `json:"request_id,omitempty"`
}

type UserEnvelope struct {
	Data      UserResponse `json:"data"`
	RequestID string       `json:"request_id,omitempty"`
}

type LoginEnvelope struct {
	Data      LoginResponse `json:"data"`
	RequestID string        `json:"request_id,omitempty"`
}

type MonitorEnvelope struct {
	Data      MonitorResponse `json:"data"`
	RequestID string          `json:"request_id,omitempty"`
}

type MonitorListEnvelope struct {
	Data      []MonitorResponse `json:"data"`
	RequestID string            `json:"request_id,omitempty"`
}

type PostListEnvelope struct {
	Data      []content.PostSummary `json:"data"`
	RequestID string                `json:"request_id,omitempty"`
}

type TopicListEnvelope struct {
	Data      []topic.TopicSummary `json:"data"`
	RequestID string               `json:"request_id,omitempty"`
}

type TrendListEnvelope struct {
	Data      []trend.TrendPoint `json:"data"`
	RequestID string             `json:"request_id,omitempty"`
}

type NotificationListEnvelope struct {
	Data      []NotificationResponse `json:"data"`
	RequestID string                 `json:"request_id,omitempty"`
}

type MarkNotificationReadResponse struct {
	Read bool `json:"read" example:"true"`
}

type MarkNotificationReadEnvelope struct {
	Data      MarkNotificationReadResponse `json:"data"`
	RequestID string                       `json:"request_id,omitempty"`
}
