package database

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"
)

// EventAnnotationModel is the GORM model for event_annotations.
type EventAnnotationModel struct {
	ID                int64  `gorm:"column:id;primaryKey"`
	EventID           int64  `gorm:"column:event_id;uniqueIndex"`
	ManualTags        string `gorm:"column:manual_tags;type:jsonb"`
	AnalystConclusion string `gorm:"column:analyst_conclusion"`
}

func (EventAnnotationModel) TableName() string { return "event_annotations" }

// EventAnnotationRepo handles writes to the event_annotations sidecar table.
type EventAnnotationRepo struct {
	db *gorm.DB
}

// NewEventAnnotationRepo creates a new EventAnnotationRepo.
func NewEventAnnotationRepo(db *gorm.DB) *EventAnnotationRepo {
	return &EventAnnotationRepo{db: db}
}

// SetManualTags sets the manual_tags field for an event.
func (r *EventAnnotationRepo) SetManualTags(ctx context.Context, eventID int64, tags []string) error {
	b, _ := json.Marshal(tags)
	return r.db.WithContext(ctx).Raw(
		`INSERT INTO event_annotations (event_id, manual_tags, analyst_conclusion)
		 VALUES (?, ?, '')
		 ON CONFLICT (event_id) DO UPDATE SET manual_tags = EXCLUDED.manual_tags`,
		eventID, string(b),
	).Error
}

// SetAnalystConclusion sets the analyst_conclusion field for an event.
func (r *EventAnnotationRepo) SetAnalystConclusion(ctx context.Context, eventID int64, conclusion string) error {
	return r.db.WithContext(ctx).Raw(
		`INSERT INTO event_annotations (event_id, manual_tags, analyst_conclusion)
		 VALUES (?, '[]', ?)
		 ON CONFLICT (event_id) DO UPDATE SET analyst_conclusion = EXCLUDED.analyst_conclusion`,
		eventID, conclusion,
	).Error
}
