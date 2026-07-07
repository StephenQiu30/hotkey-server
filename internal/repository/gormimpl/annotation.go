package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"gorm.io/gorm"
)

type AnnotationRepo struct {
	db *gorm.DB
}

func NewAnnotationRepo(db *gorm.DB) *AnnotationRepo {
	return &AnnotationRepo{db: db}
}

func (r *AnnotationRepo) UpsertEventAnnotation(ctx context.Context, eventID int64, manualTags, analystConclusion string) error {
	m := EventAnnotation{
		EventID:           eventID,
		ManualTags:        pkg.JSONB[string]{Data: manualTags},
		AnalystConclusion: analystConclusion,
	}
	return r.db.WithContext(ctx).Where("event_id = ?", eventID).Assign(m).FirstOrCreate(&EventAnnotation{}).Error
}

func (r *AnnotationRepo) UpsertTopicAnnotation(ctx context.Context, topicID int64, materialStatus, manualSummary string) error {
	m := TopicAnnotation{
		TopicID:       topicID,
		MaterialStatus: materialStatus,
		ManualSummary: manualSummary,
	}
	return r.db.WithContext(ctx).Where("topic_id = ?", topicID).Assign(m).FirstOrCreate(&TopicAnnotation{}).Error
}
