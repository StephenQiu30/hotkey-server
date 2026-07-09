package repository

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"gorm.io/gorm"
)

// SnapshotRepo handles writes to snapshot tables.
type SnapshotRepo struct {
	db *gorm.DB
}

func NewSnapshotRepo(db *gorm.DB) *SnapshotRepo {
	return &SnapshotRepo{db: db}
}

// CreateTopicSnapshot persists a topic snapshot.
func (r *SnapshotRepo) CreateTopicSnapshot(ctx context.Context, snap *entity.TopicSnapshot) error {
	return r.db.WithContext(ctx).Create(snap).Error
}

// CreateMonitorSnapshot persists a monitor snapshot.
func (r *SnapshotRepo) CreateMonitorSnapshot(ctx context.Context, snap *entity.MonitorSnapshot) error {
	return r.db.WithContext(ctx).Create(snap).Error
}

// GetTopicSnapshotBefore retrieves the most recent snapshot before a given time.
func (r *SnapshotRepo) GetTopicSnapshotBefore(ctx context.Context, topicID int64, before time.Time) (*entity.TopicSnapshot, error) {
	var snap entity.TopicSnapshot
	err := r.db.WithContext(ctx).
		Where("topic_id = ? AND snapshot_time < ?", topicID, before).
		Order("snapshot_time DESC").
		First(&snap).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}
