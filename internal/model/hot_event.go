package model

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// HotEvent is a cross-platform trending event.
type HotEvent struct {
	ID          int64
	Name        string
	HeatScore   float64
	Platform    string
	Trend       string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	PeakAt      *time.Time
	TopicIDs    pkg.Int64Array
	PostIDs     pkg.Int64Array
	Summary     string
	Category    string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EventPlatform represents a hot event's entry on one platform.
type EventPlatform struct {
	Platform  string
	Rank      int
	Title     string
	URL       string
	Heat      float64
	UpdatedAt time.Time
}
