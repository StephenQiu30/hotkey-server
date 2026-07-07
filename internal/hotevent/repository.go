package hotevent

import (
	"context"
	"time"
)

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
	Create(ctx context.Context, event *HotEvent) error
	GetByID(ctx context.Context, id int64) (*HotEvent, error)
	List(ctx context.Context, filter ListFilter) ([]*HotEvent, int64, error)
	Update(ctx context.Context, event *HotEvent) error
	ArchiveOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	AddPlatform(ctx context.Context, eventID int64, platform *EventPlatform) error
	GetPlatforms(ctx context.Context, eventID int64) ([]*EventPlatform, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}
