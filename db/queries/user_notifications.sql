-- name: CreateNotification :one
INSERT INTO user_notifications (user_id, alert_id, channel, delivery_status)
VALUES (sqlc.arg(user_id), sqlc.arg(alert_id), sqlc.arg(channel), sqlc.arg(delivery_status))
RETURNING id, user_id, alert_id, channel, delivery_status, read_at, sent_at, created_at;

-- name: ListUnreadNotifications :many
SELECT id, user_id, alert_id, channel, delivery_status, read_at, sent_at, created_at
FROM user_notifications
WHERE user_id = sqlc.arg(user_id) AND read_at IS NULL
ORDER BY created_at DESC;

-- name: MarkNotificationRead :exec
UPDATE user_notifications
SET read_at = now()
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND read_at IS NULL;
