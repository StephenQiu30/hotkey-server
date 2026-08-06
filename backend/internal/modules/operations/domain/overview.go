package domain

import "time"

// RuntimeOverview is a safe operational projection. It intentionally exposes
// queue counts and age only; River args, payloads and provider errors remain
// private to the queue infrastructure.
type RuntimeOverview struct {
	GeneratedAt       time.Time  `json:"generated_at"`
	AvailableJobs     int64      `json:"available_jobs"`
	RunningJobs       int64      `json:"running_jobs"`
	CompletedJobs     int64      `json:"completed_jobs"`
	DiscardedJobs     int64      `json:"discarded_jobs"`
	CancelledJobs     int64      `json:"cancelled_jobs"`
	OldestAvailableAt *time.Time `json:"oldest_available_at,omitempty"`
}
