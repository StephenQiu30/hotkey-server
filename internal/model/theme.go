package model

import "time"

// Theme is a thematic grouping of events/topics.
type Theme struct {
	ID        int64
	MonitorID int64
	ThemeKey  string
	Title     string
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
}
