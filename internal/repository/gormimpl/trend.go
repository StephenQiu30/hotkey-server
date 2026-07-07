package gormimpl

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"gorm.io/gorm"
)

type TrendRepo struct {
	db *gorm.DB
}

func NewTrendRepo(db *gorm.DB) *TrendRepo {
	return &TrendRepo{db: db}
}

func (r *TrendRepo) SaveTopicSnapshot(ctx context.Context, snap model.TopicSnapshot) error {
	m := TopicSnapshot{
		TopicID:           snap.TopicID,
		SnapshotTime:      snap.SnapshotTime,
		PostCount:         snap.PostCount,
		UniqueAuthorCount: snap.UniqueAuthorCount,
		EngagementSum:     snap.EngagementSum,
		HeatScore:         snap.HeatScore,
		TrendVelocity:     snap.TrendVelocity,
	}
	return r.db.WithContext(ctx).Create(&m).Error
}

func (r *TrendRepo) SaveMonitorSnapshot(ctx context.Context, snap model.MonitorSnapshot) error {
	m := MonitorSnapshot{
		MonitorID:        snap.MonitorID,
		SnapshotTime:     snap.SnapshotTime,
		NewPostCount:     snap.NewPostCount,
		ActiveTopicCount: snap.ActiveTopicCount,
		TotalEngagement:  snap.TotalEngagement,
		TopTopicID:       snap.TopTopicID,
	}
	return r.db.WithContext(ctx).Create(&m).Error
}

func (r *TrendRepo) GetPreviousTopicHeat(ctx context.Context, topicID int64) (float64, error) {
	var m TopicSnapshot
	if err := r.db.WithContext(ctx).
		Where("topic_id = ?", topicID).
		Order("snapshot_time DESC").
		First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	return m.HeatScore, nil
}

func (r *TrendRepo) GetTopicTrends(ctx context.Context, topicID int64, since time.Time) ([]model.TopicSnapshot, error) {
	var models []TopicSnapshot
	if err := r.db.WithContext(ctx).
		Where("topic_id = ? AND snapshot_time >= ?", topicID, since).
		Order("snapshot_time ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]model.TopicSnapshot, len(models))
	for i := range models {
		result[i] = ToTopicSnapshot(models[i])
	}
	return result, nil
}

func (r *TrendRepo) GetMonitorTrends(ctx context.Context, monitorID int64, since time.Time) ([]model.MonitorSnapshot, error) {
	var models []MonitorSnapshot
	if err := r.db.WithContext(ctx).
		Where("monitor_id = ? AND snapshot_time >= ?", monitorID, since).
		Order("snapshot_time ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]model.MonitorSnapshot, len(models))
	for i := range models {
		result[i] = ToMonitorSnapshot(models[i])
	}
	return result, nil
}
