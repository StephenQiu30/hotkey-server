package model

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// KeywordMonitor is a keyword monitoring task.
type KeywordMonitor struct {
	ID                    int64
	UserID                int64
	Name                  string
	QueryText             string
	Language              string
	Region                string
	Status                string
	PollIntervalMinutes   int
	AlertEnabled          bool
	AlertThresholdConfig  pkg.JSONB[map[string]any]
	LastPolledAt          *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// CreateMonitorInput is the input for creating a monitor.
type CreateMonitorInput struct {
	Name                string
	QueryText           string
	Language            string
	Region              string
	PollIntervalMinutes int
	AlertEnabled        bool
}

// UpdateMonitorInput is the input for updating a monitor.
type UpdateMonitorInput struct {
	Name                *string
	QueryText           *string
	Language            *string
	Region              *string
	PollIntervalMinutes *int
	AlertEnabled        *bool
	Status              *string
}
