package database

import (
	"context"
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/notify"
)

// NotifyRepo implements notify.Repository using PostgreSQL.
type NotifyRepo struct {
	db *sql.DB
}

// NewNotifyRepo creates a new Postgres-backed notification repository.
func NewNotifyRepo(db *sql.DB) *NotifyRepo {
	return &NotifyRepo{db: db}
}

func (r *NotifyRepo) ListUnread(ctx context.Context, userID int64) ([]notify.Notification, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, alert_id, channel, delivery_status, read_at, sent_at, created_at
		 FROM user_notifications WHERE user_id = $1 AND read_at IS NULL ORDER BY created_at DESC`, userID,
	)
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
	res, err := r.db.ExecContext(ctx,
		`UPDATE user_notifications SET read_at = now() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		notificationID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return notify.ErrNotFound
	}
	return nil
}

func (r *NotifyRepo) Create(ctx context.Context, n notify.Notification) (notify.Notification, error) {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO user_notifications (user_id, alert_id, channel, delivery_status)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, alert_id, channel, delivery_status, read_at, sent_at, created_at`,
		n.UserID, n.AlertID, n.Channel, n.DeliveryStatus,
	).Scan(&n.ID, &n.UserID, &n.AlertID, &n.Channel, &n.DeliveryStatus, &n.ReadAt, &n.SentAt, &n.CreatedAt)
	return n, err
}
