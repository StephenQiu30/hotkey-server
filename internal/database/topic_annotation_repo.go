package database

import (
	"context"

	"gorm.io/gorm"
)

// TopicAnnotationModel is the GORM model for topic_annotations.
type TopicAnnotationModel struct {
	ID             int64  `gorm:"column:id;primaryKey"`
	TopicID        int64  `gorm:"column:topic_id;uniqueIndex"`
	MaterialStatus string `gorm:"column:material_status"`
}

func (TopicAnnotationModel) TableName() string { return "topic_annotations" }

// TopicAnnotationRepo handles writes to the topic_annotations sidecar table.
type TopicAnnotationRepo struct {
	db *gorm.DB
}

// NewTopicAnnotationRepo creates a new TopicAnnotationRepo.
func NewTopicAnnotationRepo(db *gorm.DB) *TopicAnnotationRepo {
	return &TopicAnnotationRepo{db: db}
}

// SetMaterialStatus sets the material_status field for a topic.
func (r *TopicAnnotationRepo) SetMaterialStatus(ctx context.Context, topicID int64, status string) error {
	return r.db.WithContext(ctx).Raw(
		`INSERT INTO topic_annotations (topic_id, material_status)
		 VALUES (?, ?)
		 ON CONFLICT (topic_id) DO UPDATE SET material_status = EXCLUDED.material_status`,
		topicID, status,
	).Error
}
