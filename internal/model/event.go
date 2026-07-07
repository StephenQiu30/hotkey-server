package model

import "time"

// Event is a knowledge-platform event built from monitored posts.
type Event struct {
	ID            int64
	MonitorID     int64
	EventKey      string
	Title         string
	Summary       string
	MachineStatus string
	SourcePostID  *int64
	FirstSeenAt   time.Time
	LastActiveAt  time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
