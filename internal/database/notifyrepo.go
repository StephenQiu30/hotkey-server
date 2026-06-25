package database

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"gorm.io/gorm"
)

// NotifyRepo implements notify.Repository using PostgreSQL via GORM.
type NotifyRepo struct {
	db *gorm.DB
}

// NewNotifyRepo creates a new Postgres-backed notification repository.
func NewNotifyRepo(db *gorm.DB) *NotifyRepo {
	return &NotifyRepo{db: db}
}

func (r *NotifyRepo) ListUnread(ctx context.Context, userID int64) ([]notify.Notification, error) {
	rows, err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, alert_id, channel, delivery_status, read_at, sent_at, created_at
		 FROM user_notifications WHERE user_id = ? AND read_at IS NULL ORDER BY created_at DESC`, userID,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []notify.Notification
	for rows.Next() {
		var n notify.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.AlertID, &n.Channel, &n.DeliveryStatus, &n.ReadAt, &n.SentAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

func (r *NotifyRepo) MarkRead(ctx context.Context, userID, notificationID int64) error {
	result := r.db.WithContext(ctx).Exec(
		`UPDATE user_notifications SET read_at = now() WHERE id = ? AND user_id = ? AND read_at IS NULL`,
		notificationID, userID,
	)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return notify.ErrNotFound
	}
	return nil
}

func (r *NotifyRepo) Create(ctx context.Context, n notify.Notification) (notify.Notification, error) {
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO user_notifications (user_id, alert_id, channel, delivery_status)
		 VALUES (?, ?, ?, ?)
		 RETURNING id, user_id, alert_id, channel, delivery_status, read_at, sent_at, created_at`,
		n.UserID, n.AlertID, n.Channel, n.DeliveryStatus,
	).Scan(&n).Error
	return n, err
}
