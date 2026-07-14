package dto

import "time"

// UserContext carries the authenticated user's full information.
// Injected into request context by UserContextMiddleware.
type UserContext struct {
	UserID      int64     `json:"user_id"`
	SessionID   int64     `json:"session_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	PlanType    string    `json:"plan_type"`
	IPAddress   string    `json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
