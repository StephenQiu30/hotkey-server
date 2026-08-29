package domain

import "time"

// RuntimeAlert is an actionable, bounded projection of one operational rule.
// It never contains River args, provider errors, source values, or user input.
type RuntimeAlert struct {
	AlertID          string    `json:"alert_id"`
	PolicyVersion    string    `json:"policy_version"`
	Severity         string    `json:"severity"`
	ReasonCode       string    `json:"reason_code"`
	RunbookURL       string    `json:"runbook_url"`
	Owner            string    `json:"owner"`
	SilenceKey       string    `json:"silence_key"`
	ThresholdCount   int64     `json:"threshold_count"`
	ThresholdSeconds int64     `json:"threshold_seconds"`
	JobID            int64     `json:"job_id,omitempty"`
	EventID          int64     `json:"event_id,omitempty"`
	TraceID          string    `json:"trace_id,omitempty"`
	AttemptID        int64     `json:"attempt_id,omitempty"`
	NotificationID   int64     `json:"notification_id,omitempty"`
	ResourceType     string    `json:"resource_type,omitempty"`
	ResourceID       int64     `json:"resource_id,omitempty"`
	AffectedCount    int64     `json:"affected_count"`
	TriggeredAt      time.Time `json:"triggered_at"`
}

// RuntimeOverview is a safe operational projection. It intentionally exposes
// queue counts, age and fixed alert rules only; River args, payloads and
// provider errors remain private to the queue infrastructure.
type RuntimeOverview struct {
	GeneratedAt        time.Time      `json:"generated_at"`
	AlertPolicyVersion string         `json:"alert_policy_version"`
	AvailableJobs      int64          `json:"available_jobs"`
	RunningJobs        int64          `json:"running_jobs"`
	CompletedJobs      int64          `json:"completed_jobs"`
	DiscardedJobs      int64          `json:"discarded_jobs"`
	CancelledJobs      int64          `json:"cancelled_jobs"`
	QueueLagSeconds    float64        `json:"queue_lag_seconds"`
	OldestAvailableAt  *time.Time     `json:"oldest_available_at,omitempty"`
	Alerts             []RuntimeAlert `json:"alerts"`
}
