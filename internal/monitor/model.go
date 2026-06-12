package monitor

import "time"

type Monitor struct {
	ID                  int64     `json:"id"`
	UserID              int64     `json:"user_id"`
	Name                string    `json:"name"`
	QueryText           string    `json:"query_text"`
	Language            string    `json:"language"`
	Region              string    `json:"region"`
	Status              string    `json:"status"`
	PollIntervalMinutes int       `json:"poll_interval_minutes"`
	AlertEnabled        bool      `json:"alert_enabled"`
	AlertThresholdCfg   any       `json:"alert_threshold_config"`
	LastPolledAt        *time.Time `json:"last_polled_at,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}
