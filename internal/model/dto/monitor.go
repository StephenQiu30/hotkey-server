package dto

import "time"

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
