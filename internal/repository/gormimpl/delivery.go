package gormimpl

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type DeliveryRepo struct {
	db *gorm.DB
}

func NewDeliveryRepo(db *gorm.DB) *DeliveryRepo {
	return &DeliveryRepo{db: db}
}

func (r *DeliveryRepo) Create(ctx context.Context, notificationID int64, recipientEmail, provider string) (int64, error) {
	m := EmailDelivery{
		NotificationID: notificationID,
		RecipientEmail: recipientEmail,
		Provider:       provider,
		Status:         "pending",
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (r *DeliveryRepo) UpdateStatus(ctx context.Context, id int64, status, providerMessageID, errorMessage string, sentAt *time.Time) error {
	updates := map[string]any{
		"status":    status,
		"error_message": errorMessage,
	}
	if status == "sent" {
		updates["provider_message_id"] = providerMessageID
		updates["sent_at"] = sentAt
	}
	return r.db.WithContext(ctx).Model(&EmailDelivery{}).Where("id = ?", id).Updates(updates).Error
}
