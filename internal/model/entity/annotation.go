package entity

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

type EventAnnotation struct {
	ID                int64             `gorm:"column:id;primaryKey"`
	EventID           int64             `gorm:"column:event_id"`
	ManualTags        pkg.JSONB[string] `gorm:"column:manual_tags;type:jsonb"`
	AnalystConclusion string            `gorm:"column:analyst_conclusion"`
	CreatedAt         time.Time         `gorm:"column:created_at"`
	UpdatedAt         time.Time         `gorm:"column:updated_at"`
}

func (EventAnnotation) TableName() string { return "event_annotations" }

type TopicAnnotation struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	TopicID        int64     `gorm:"column:topic_id"`
	MaterialStatus string    `gorm:"column:material_status"`
	ManualSummary  string    `gorm:"column:manual_summary"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (TopicAnnotation) TableName() string { return "topic_annotations" }
