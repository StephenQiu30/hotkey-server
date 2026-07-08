package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"gorm.io/gorm"
)

// NotifyRepo implements notify.Repository via GORM.
type NotifyRepo struct {
	db *gorm.DB
}

func NewNotifyRepo(db *gorm.DB) *NotifyRepo {
	return &NotifyRepo{db: db}
}

func (r *NotifyRepo) ListUnread(ctx context.Context, userID int64) ([]notify.Notification, error) {
	var models []entity.UserNotification
	if err := r.db.WithContext(ctx).Where("user_id = ? AND read_at IS NULL", userID).
		Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]notify.Notification, len(models))
	for i := range models {
		result[i] = toDomainNotification(models[i])
	}
	return result, nil
}

func (r *NotifyRepo) MarkRead(ctx context.Context, userID, notificationID int64) error {
	result := r.db.WithContext(ctx).Model(&entity.UserNotification{}).
		Where("id = ? AND user_id = ?", notificationID, userID).
		Update("read_at", gorm.Expr("now()"))
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func (r *NotifyRepo) Create(ctx context.Context, n notify.Notification) (notify.Notification, error) {
	m := entity.UserNotification{
		UserID:         n.UserID,
		AlertID:        n.AlertID,
		Channel:        n.Channel,
		DeliveryStatus: n.DeliveryStatus,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return notify.Notification{}, err
	}
	n.ID = m.ID
	n.CreatedAt = m.CreatedAt
	return n, nil
}

func toDomainNotification(m entity.UserNotification) notify.Notification {
	return notify.Notification{
		ID:             m.ID,
		UserID:         m.UserID,
		AlertID:        m.AlertID,
		Channel:        m.Channel,
		DeliveryStatus: m.DeliveryStatus,
		ReadAt:         m.ReadAt,
		SentAt:         m.SentAt,
		CreatedAt:      m.CreatedAt,
	}
}
