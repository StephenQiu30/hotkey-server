package repository

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/convert"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"gorm.io/gorm"
)

// NotifyRepo implements notify.Repository via GORM.
type NotifyRepo struct {
	db *gorm.DB
}

func NewNotifyRepo(db *gorm.DB) *NotifyRepo {
	return &NotifyRepo{db: db}
}

func (r *NotifyRepo) ListUnread(ctx context.Context, userID int64) ([]dto.Notification, error) {
	var models []entity.UserNotification
	if err := r.db.WithContext(ctx).Where("user_id = ? AND read_at IS NULL", userID).
		Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]dto.Notification, len(models))
	for i := range models {
		result[i] = convert.NotificationEntityToDTO(models[i])
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

func (r *NotifyRepo) Create(ctx context.Context, n dto.Notification) (dto.Notification, error) {
	m := entity.UserNotification{
		UserID:         n.UserID,
		AlertID:        n.AlertID,
		Channel:        n.Channel,
		DeliveryStatus: n.DeliveryStatus,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return dto.Notification{}, err
	}
	n.ID = m.ID
	n.CreatedAt = m.CreatedAt
	return n, nil
}
