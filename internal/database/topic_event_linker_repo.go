package database

import (
	"context"

	"gorm.io/gorm"
)

// TopicEventLinkerRepo implements topic.TopicEventLinker by writing to topic_events.
type TopicEventLinkerRepo struct {
	db *gorm.DB
}

// NewTopicEventLinkerRepo creates a new TopicEventLinkerRepo.
func NewTopicEventLinkerRepo(db *gorm.DB) *TopicEventLinkerRepo {
	return &TopicEventLinkerRepo{db: db}
}

// LinkEvent inserts a topic-event association row.
func (r *TopicEventLinkerRepo) LinkEvent(ctx context.Context, topicID, eventID int64) error {
	link := TopicEvent{
		TopicID:          topicID,
		EventID:          eventID,
		RelationshipType: "member",
	}
	return r.db.WithContext(ctx).Create(&link).Error
}
