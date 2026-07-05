package hotevent

import "time"

// ListFilter defines filtering and pagination for List queries.
type ListFilter struct {
	Status   string
	Platform string
	Sort     string // "heat_score" (default) or "last_seen"
	Limit    int
	Offset   int
}

// Repository defines persistence operations for HotEvent.
type Repository interface {
	Create(event *HotEvent) error
	GetByID(id int64) (*HotEvent, error)
	List(filter ListFilter) ([]*HotEvent, int64, error)
	Update(event *HotEvent) error
	ArchiveOlderThan(cutoff time.Time) (int64, error)
	AddPlatform(eventID int64, platform *EventPlatform) error
	GetPlatforms(eventID int64) ([]*EventPlatform, error)
	DeleteOlderThan(cutoff time.Time) (int64, error)
}
