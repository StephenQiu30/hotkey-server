package repository

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"gorm.io/gorm"
)

// TopicWriteRepo handles writes to topics and topic_posts tables.
type TopicWriteRepo struct {
	db *gorm.DB
}

func NewTopicWriteRepo(db *gorm.DB) *TopicWriteRepo {
	return &TopicWriteRepo{db: db}
}

// CreateTopic inserts a new topic and returns its ID.
func (r *TopicWriteRepo) CreateTopic(ctx context.Context, monitorID int64, topicKey, title, summary string) (int64, error) {
	t := entity.Topic{
		MonitorID:       monitorID,
		TopicKey:        topicKey,
		Title:           title,
		Summary:         summary,
		Status:          "active",
		FirstDetectedAt: time.Now(),
		LastActiveAt:    time.Now(),
	}
	if err := r.db.WithContext(ctx).Create(&t).Error; err != nil {
		return 0, err
	}
	return t.ID, nil
}

// AddTopicPost links a post to a topic.
func (r *TopicWriteRepo) AddTopicPost(ctx context.Context, topicID, postID int64, score float64) error {
	tp := entity.TopicPost{
		TopicID:         topicID,
		PostID:          postID,
		MembershipScore: score,
		AddedAt:         time.Now(),
	}
	return r.db.WithContext(ctx).Where("topic_id = ? AND post_id = ?", topicID, postID).
		FirstOrCreate(&tp).Error
}

// UpdateTopicHeat updates a topic's heat score and last active time.
func (r *TopicWriteRepo) UpdateTopicHeat(ctx context.Context, topicID int64, heat float64, direction string) error {
	return r.db.WithContext(ctx).Model(&entity.Topic{}).
		Where("id = ?", topicID).
		Updates(map[string]interface{}{
			"current_heat_score": heat,
			"trend_direction":    direction,
			"last_active_at":     time.Now(),
			"updated_at":         time.Now(),
		}).Error
}
