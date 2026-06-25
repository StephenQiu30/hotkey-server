package database

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"gorm.io/gorm"
)

// DeliveryRepo implements jobs.DeliveryRepository via GORM.
type DeliveryRepo struct {
	db *gorm.DB
}

func NewDeliveryRepo(db *gorm.DB) *DeliveryRepo {
	return &DeliveryRepo{db: db}
}

func (r *DeliveryRepo) CreateDelivery(ctx context.Context, d jobs.EmailDelivery) error {
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO email_deliveries (notification_id, recipient_email, provider, status)
		 VALUES (?, ?, ?, ?)`,
		d.NotificationID, d.RecipientEmail, d.Provider, d.Status,
	).Error
}

func (r *DeliveryRepo) UpdateDeliveryStatus(ctx context.Context, notificationID int64, status string, providerMsgID string, errMsg string) error {
	return r.db.WithContext(ctx).Exec(
		`UPDATE email_deliveries SET status = ?, provider_message_id = ?, error_message = ?
		 WHERE notification_id = ?`,
		status, providerMsgID, errMsg, notificationID,
	).Error
}

func (r *DeliveryRepo) GetPendingDeliveries(ctx context.Context, limit int) ([]jobs.EmailDelivery, error) {
	rows, err := r.db.WithContext(ctx).Raw(
		`SELECT id, notification_id, recipient_email, provider, provider_message_id, status, error_message, sent_at
		 FROM email_deliveries WHERE status = 'pending' ORDER BY id ASC LIMIT ?`, limit,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []jobs.EmailDelivery
	for rows.Next() {
		var d jobs.EmailDelivery
		if err := rows.Scan(&d.ID, &d.NotificationID, &d.RecipientEmail, &d.Provider, &d.ProviderMessageID, &d.Status, &d.ErrorMessage, &d.SentAt); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

func (r *DeliveryRepo) ResolveEmail(ctx context.Context, notificationID int64) (string, error) {
	var email string
	err := r.db.WithContext(ctx).Raw(
		`SELECT u.email FROM user_notifications un
		 JOIN users u ON u.id = un.user_id
		 WHERE un.id = ?`, notificationID,
	).Scan(&email).Error
	return email, err
}
