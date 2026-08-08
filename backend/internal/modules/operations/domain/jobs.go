package domain

import (
	"fmt"
	"strings"
	"time"
)

type JobState string

const (
	JobAvailable JobState = "available"
	JobRunning   JobState = "running"
	JobCompleted JobState = "completed"
	JobDiscarded JobState = "discarded"
	JobCancelled JobState = "cancelled"
)

func (state JobState) Valid() bool {
	switch state {
	case JobAvailable, JobRunning, JobCompleted, JobDiscarded, JobCancelled:
		return true
	default:
		return false
	}
}

type JobSummary struct {
	ID          int64      `json:"id"`
	Kind        string     `json:"kind"`
	ResourceID  int64      `json:"resource_id,omitempty"`
	State       JobState   `json:"state"`
	FailureCode string     `json:"failure_code,omitempty"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"max_attempts"`
	Priority    int        `json:"priority"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	AttemptedAt *time.Time `json:"attempted_at,omitempty"`
	FinalizedAt *time.Time `json:"finalized_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (job JobSummary) Validate() error {
	if job.ID <= 0 || strings.TrimSpace(job.Kind) == "" || job.ResourceID < 0 || !job.State.Valid() || !validJobFailureCode(job.FailureCode) || job.Attempt < 0 || job.MaxAttempts < 1 || job.Priority < 1 || job.ScheduledAt.IsZero() || job.CreatedAt.IsZero() {
		return fmt.Errorf("invalid job summary")
	}
	return nil
}

func validJobFailureCode(code string) bool {
	return code == "" || code == "retryable" || code == "permanent" || code == "cancelled"
}

type JobListQuery struct {
	Cursor int64
	Kind   string
	State  JobState
	Limit  int
}

func (query JobListQuery) Validate() error {
	if query.Cursor < 0 || query.Limit < 1 || query.Limit > 100 || query.State != "" && !query.State.Valid() || len(query.Kind) > 64 {
		return fmt.Errorf("invalid job list query")
	}
	return nil
}

type JobPage struct {
	Items      []JobSummary `json:"items"`
	NextCursor int64        `json:"next_cursor,omitempty"`
}

type JobMutationInput struct {
	ActorID int64
	JobID   int64
}

func (input JobMutationInput) Validate() error {
	if input.ActorID <= 0 || input.JobID <= 0 {
		return fmt.Errorf("job actor and id are required")
	}
	return nil
}
