package database

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// CreateEventInput holds parameters for creating a new event.
type CreateEventInput struct {
	MonitorID     int64
	EventKey      string
	Title         string
	Summary       string
	MachineStatus string
	SourcePostID  *int64
	FirstSeenAt   time.Time
	LastActiveAt  time.Time
}

// EventRepo manages events in PostgreSQL.
type EventRepo struct {
	db *gorm.DB
}

// NewEventRepo creates a new EventRepo.
func NewEventRepo(db *gorm.DB) *EventRepo {
	return &EventRepo{db: db}
}

// CreateEvent inserts a new event row and returns its ID.
func (r *EventRepo) CreateEvent(ctx context.Context, in CreateEventInput) (int64, error) {
	event := Event{
		MonitorID:     in.MonitorID,
		EventKey:      in.EventKey,
		Title:         in.Title,
		Summary:       in.Summary,
		MachineStatus: in.MachineStatus,
		SourcePostID:  in.SourcePostID,
		FirstSeenAt:   in.FirstSeenAt,
		LastActiveAt:  in.LastActiveAt,
	}
	if err := r.db.WithContext(ctx).Create(&event).Error; err != nil {
		return 0, err
	}
	return event.ID, nil
}

// ListEventsByMonitor returns all events for a given monitor.
func (r *EventRepo) ListEventsByMonitor(ctx context.Context, monitorID int64) ([]Event, error) {
	var events []Event
	if err := r.db.WithContext(ctx).Where("monitor_id = ?", monitorID).Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}
