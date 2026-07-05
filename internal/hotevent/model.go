// Package hotevent defines the HotEvent domain model — the top-level
// "hot topic / trending event" entity aggregated across platforms.
package hotevent

import "time"

// Status constants for HotEvent lifecycle.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Trend direction constants.
const (
	TrendRising    = "rising"
	TrendStable    = "stable"
	TrendDeclining = "declining"
)

// HotEvent represents a cross-platform hot topic or trending event.
//
// The event originates from either X Topic clustering (existing Jaccard pipeline)
// or platform trending lists (weibo / zhihu / baidu), and is matched/merged
// by the aggregator using cosine similarity + keyword overlap.
type HotEvent struct {
	ID          int64
	Name        string     // Event title (auto-generated or LLM summary)
	HeatScore   float64    // Composite heat score across platforms
	Platform    string     // Primary platform, "multi" for cross-platform
	Trend       string     // rising / stable / declining
	FirstSeenAt time.Time  // First observation
	LastSeenAt  time.Time  // Most recent observation
	PeakAt      *time.Time // Time of peak heat (nil if unknown)
	TopicIDs    []int64    // Referenced X Topic IDs
	PostIDs     []int64    // Referenced post IDs (platform_posts table)
	Summary     string     // AI-generated summary
	Category    string     // Category label (tech, politics, etc.)
	Status      string     // active / archived
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
