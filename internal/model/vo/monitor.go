package vo

// MonitorData is the JSON representation of a monitor.
type MonitorData struct {
	ID                  int64  `json:"id"`
	UserID              int64  `json:"user_id"`
	Name                string `json:"name"`
	QueryText           string `json:"query_text"`
	Language            string `json:"language"`
	Region              string `json:"region"`
	Status              string `json:"status"`
	PollIntervalMinutes int    `json:"poll_interval_minutes"`
	AlertEnabled        bool   `json:"alert_enabled"`
}
