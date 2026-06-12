package monitor

import (
	"errors"
	"time"
)

// Sentinel errors for monitor operations.
var (
	ErrInvalidInterval  = errors.New("poll interval must be one of: 5, 10, 15, 30 minutes")
	ErrInvalidInput     = errors.New("invalid input")
	ErrNotFound         = errors.New("monitor not found")
	ErrForbidden        = errors.New("not authorized")
)

// AllowedIntervals defines valid poll interval values in minutes.
var AllowedIntervals = map[int]struct{}{5: {}, 10: {}, 15: {}, 30: {}}

// Monitor represents a keyword monitoring task.
type Monitor struct {
	ID                   int64
	UserID               int64
	Name                 string
	QueryText            string
	Language             string
	Region               string
	Status               string
	PollIntervalMinutes  int
	AlertEnabled         bool
	AlertThresholdConfig map[string]interface{}
	LastPolledAt         *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// CreateMonitorInput holds data for creating a monitor.
type CreateMonitorInput struct {
	Name                string
	QueryText           string
	Language            string
	Region              string
	PollIntervalMinutes int
	AlertEnabled        bool
}

// UpdateMonitorInput holds data for updating a monitor.
type UpdateMonitorInput struct {
	Name                *string
	QueryText           *string
	Language            *string
	Region              *string
	PollIntervalMinutes *int
	AlertEnabled        *bool
	Status              *string
}
