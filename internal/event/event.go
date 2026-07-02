// Package event defines the Event domain model and service for the knowledge
// middle-platform. An Event represents a single occurrence or development
// within a hot topic, aggregated from one or more platform posts.
package event

import (
	"time"
)

// Event is the core domain object for the knowledge middle-platform.
type Event struct {
	ID            int64
	MonitorID     int64
	TopicID       int64
	EventKey      string
	Title         string
	Summary       string
	MachineStatus string
	FirstSeenAt   time.Time
	LastActiveAt  time.Time
}

// PostFact represents a post used to build an event.
type PostFact struct {
	PostID      int64
	PublishedAt time.Time
	Text        string
}

// BuildEventInput holds parameters for building an event from posts.
type BuildEventInput struct {
	MonitorID  int64
	TopicID    int64
	TopicTitle string
	EventSeed  string
	Posts      []PostFact
}
