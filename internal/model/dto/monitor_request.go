package dto

// CreateMonitorRequest is the request body for POST /api/v1/monitors.
type CreateMonitorRequest struct {
	Name                string `json:"name" example:"AI monitor"`
	QueryText           string `json:"query_text" example:"openai OR gpt"`
	Language            string `json:"language,omitempty" example:"en"`
	Region              string `json:"region,omitempty" example:"US"`
	PollIntervalMinutes int    `json:"poll_interval_minutes" example:"15"`
	AlertEnabled        bool   `json:"alert_enabled" example:"true"`
}

// UpdateMonitorRequest is the request body for PATCH /api/v1/monitors/:id.
type UpdateMonitorRequest struct {
	Name                *string `json:"name,omitempty" example:"AI monitor"`
	QueryText           *string `json:"query_text,omitempty" example:"openai OR gpt"`
	Language            *string `json:"language,omitempty" example:"en"`
	Region              *string `json:"region,omitempty" example:"US"`
	PollIntervalMinutes *int    `json:"poll_interval_minutes,omitempty" example:"15"`
	AlertEnabled        *bool   `json:"alert_enabled,omitempty" example:"true"`
	Status              *string `json:"status,omitempty" example:"active"`
}
