package model

import "time"

// KnowledgeRun is a run log for batch knowledge operations.
type KnowledgeRun struct {
	ID           int64
	RunKey       string
	RunType      string
	TargetDate   *time.Time
	Status       string
	ErrorMessage string
	StartedAt    *time.Time
	FinishedAt   *time.Time
	CreatedAt    time.Time
}
