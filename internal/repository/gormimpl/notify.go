package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"gorm.io/gorm"
)

type NotifyRepo struct {
	db *gorm.DB
}

func NewNotifyRepo(db *gorm.DB) *NotifyRepo {
	return &NotifyRepo{db: db}
}

func (r *NotifyRepo) ListUnread(ctx context.Context, userID int64) ([]model.Notification, error) {
	var models []UserNotification
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND read_at IS NULL", userID).
		Order("created_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]model.Notification, len(models))
	for i := range models {
		result[i] = ToNotification(models[i])
	}
	return result, nil
}

func (r *NotifyRepo) MarkRead(ctx context.Context, userID, notificationID int64) error {
	result := r.db.WithContext(ctx).Model(&UserNotification{}).
		Where("id = ? AND user_id = ?", notificationID, userID).
		Update("read_at", gorm.Expr("now()"))
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func (r *NotifyRepo) Create(ctx context.Context, n model.Notification) (model.Notification, error) {
	m := UserNotification{
		UserID:         n.UserID,
		AlertID:        n.AlertID,
		Channel:        n.Channel,
		DeliveryStatus: n.DeliveryStatus,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return model.Notification{}, err
	}
	n.ID = m.ID
	n.CreatedAt = m.CreatedAt
	return n, nil
}
