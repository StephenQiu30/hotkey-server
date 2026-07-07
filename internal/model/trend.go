package model

import "time"

// TopicSnapshot is a point-in-time snapshot of a topic's metrics.
type TopicSnapshot struct {
	ID               int64
	TopicID          int64
	SnapshotTime     time.Time
	PostCount        int
	UniqueAuthorCount int
	EngagementSum    int
	HeatScore        float64
	TrendVelocity    float64
}

// MonitorSnapshot is a point-in-time snapshot of a monitor's metrics.
type MonitorSnapshot struct {
	ID              int64
	MonitorID       int64
	SnapshotTime    time.Time
	NewPostCount    int
	ActiveTopicCount int
	TotalEngagement int
	TopTopicID      *int64
}
