package dto

import "time"

// HotEvent represents a cross-platform hot topic or trending event.
type HotEvent struct {
	ID          int64
	Name        string
	HeatScore   float64
	Platform    string
	Trend       string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	PeakAt      *time.Time
	TopicIDs    []int64
	PostIDs     []int64
	Summary     string
	Category    string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EventPlatform represents a HotEvent's entry on a specific platform.
type EventPlatform struct {
	Platform string
	Rank     int
	Title    string
	URL      string
	Heat     float64
}
