package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TopicRepo struct {
	db *gorm.DB
}

func NewTopicRepo(db *gorm.DB) *TopicRepo {
	return &TopicRepo{db: db}
}

func (r *TopicRepo) UpsertTopic(ctx context.Context, monitorID int64, topicKey, title, summary string, heatScore float64) (int64, error) {
	m := &Topic{
		MonitorID:        monitorID,
		TopicKey:         topicKey,
		Title:            title,
		Summary:          summary,
		CurrentHeatScore: heatScore,
	}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "monitor_id"}, {Name: "topic_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "summary", "current_heat_score", "updated_at"}),
		}).
		Create(m).Error; err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (r *TopicRepo) AddPostToTopic(ctx context.Context, topicID, postID int64, membershipScore float64) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "topic_id"}, {Name: "post_id"}},
			DoNothing: true,
		}).
		Create(&TopicPost{
			TopicID:         topicID,
			PostID:          postID,
			MembershipScore: membershipScore,
		}).Error
}

func (r *TopicRepo) ListByMonitor(ctx context.Context, monitorID int64) ([]model.TopicSummary, error) {
	var topics []Topic
	if err := r.db.WithContext(ctx).
		Where("monitor_id = ?", monitorID).
		Order("current_heat_score DESC").
		Find(&topics).Error; err != nil {
		return nil, err
	}
	result := make([]model.TopicSummary, len(topics))
	for i, t := range topics {
		result[i] = model.TopicSummary{
			ID:             t.ID,
			Title:          t.Title,
			Summary:        t.Summary,
			CurrentHeat:    t.CurrentHeatScore,
			TrendDirection: t.TrendDirection,
			PostCount:      int(t.ID), // placeholder — actual count via subquery
		}
	}
	return result, nil
}

func (r *TopicRepo) GetByID(ctx context.Context, id int64) (*model.TopicSummary, error) {
	var t Topic
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &model.TopicSummary{
		ID:             t.ID,
		Title:          t.Title,
		Summary:        t.Summary,
		CurrentHeat:    t.CurrentHeatScore,
		TrendDirection: t.TrendDirection,
	}, nil
}
