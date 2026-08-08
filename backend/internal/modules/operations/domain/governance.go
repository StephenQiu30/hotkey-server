package domain

import "time"

const (
	DimensionActiveMonitors       = "active_monitors"
	DimensionManualSearches       = "manual_searches"
	DimensionSourceCalls          = "source_calls"
	DimensionAITokens             = "ai_tokens"
	DimensionAICost               = "ai_cost"
	DimensionNotificationDelivery = "notification_deliveries"

	ActiveMonitorLimit   int64 = 50
	ManualSearchDayLimit int64 = 20
)

type UsageItem struct {
	Dimension string     `json:"dimension"`
	Label     string     `json:"label"`
	Scope     string     `json:"scope"`
	Mode      string     `json:"mode"`
	Unit      string     `json:"unit"`
	Used      string     `json:"used"`
	Limit     *string    `json:"limit,omitempty"`
	Remaining *string    `json:"remaining,omitempty"`
	Reserved  *string    `json:"reserved,omitempty"`
	Settled   *string    `json:"settled,omitempty"`
	ResetAt   *time.Time `json:"reset_at,omitempty"`
}

type UsageOverview struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Items       []UsageItem `json:"items"`
}

type AuditQuery struct {
	Cursor       int64
	Limit        int
	Action       string
	ResourceType string
	Result       string
}

type AuditRecord struct {
	ID           int64     `json:"id"`
	ActorType    string    `json:"actor_type"`
	ActorID      *int64    `json:"actor_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   *int64    `json:"resource_id,omitempty"`
	Result       string    `json:"result"`
	RequestID    string    `json:"request_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuditPage struct {
	Items      []AuditRecord `json:"items"`
	NextCursor int64         `json:"next_cursor,omitempty"`
}
