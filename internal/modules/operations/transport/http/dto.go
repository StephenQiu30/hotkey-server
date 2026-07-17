package http

import (
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
)

type JobResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type JobResponse struct {
	ID          int64      `json:"id"`
	Kind        string     `json:"kind"`
	State       string     `json:"state"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"max_attempts"`
	Priority    int        `json:"priority"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	AttemptedAt *time.Time `json:"attempted_at,omitempty"`
	FinalizedAt *time.Time `json:"finalized_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type JobPageResponse struct {
	Items      []JobResponse `json:"items"`
	NextCursor int64         `json:"next_cursor,omitempty"`
}

func jobResponse(job operationsdomain.JobSummary) JobResponse {
	return JobResponse{ID: job.ID, Kind: job.Kind, State: string(job.State), Attempt: job.Attempt, MaxAttempts: job.MaxAttempts, Priority: job.Priority, ScheduledAt: job.ScheduledAt, AttemptedAt: job.AttemptedAt, FinalizedAt: job.FinalizedAt, CreatedAt: job.CreatedAt}
}

func jobPageResponse(page operationsdomain.JobPage) JobPageResponse {
	response := JobPageResponse{Items: make([]JobResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, job := range page.Items {
		response.Items = append(response.Items, jobResponse(job))
	}
	return response
}
