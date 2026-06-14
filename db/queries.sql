-- sqlc query definitions (auth, monitor, notify)

-- users ---------------------------------------------------------------------

-- name: GetUserByEmail :one
SELECT id, email, password_hash, display_name, status, plan_type, created_at, updated_at
FROM users
WHERE email = sqlc.arg(email);

-- name: GetUserByID :one
SELECT id, email, password_hash, display_name, status, plan_type, created_at, updated_at
FROM users
WHERE id = sqlc.arg(id);

-- name: CreateUser :one
INSERT INTO users (email, password_hash, display_name)
VALUES (sqlc.arg(email), sqlc.arg(password_hash), sqlc.arg(display_name))
RETURNING id, email, password_hash, display_name, status, plan_type, created_at, updated_at;

-- name: ExistsByEmail :one
SELECT EXISTS(SELECT 1 FROM users WHERE email = sqlc.arg(email));

-- keyword_monitors ----------------------------------------------------------

-- name: CreateMonitor :one
INSERT INTO keyword_monitors (user_id, name, query_text, language, region, poll_interval_minutes, alert_enabled, alert_threshold_config)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(query_text), sqlc.arg(language), sqlc.arg(region), sqlc.arg(poll_interval_minutes), sqlc.arg(alert_enabled), sqlc.arg(alert_threshold_config))
RETURNING id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at;

-- name: GetMonitorByID :one
SELECT id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at
FROM keyword_monitors
WHERE id = sqlc.arg(id);

-- name: ListMonitorsByUser :many
SELECT id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at
FROM keyword_monitors
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at DESC;

-- name: UpdateMonitor :one
UPDATE keyword_monitors
SET name = coalesce(sqlc.narg(name), name),
    query_text = coalesce(sqlc.narg(query_text), query_text),
    language = coalesce(sqlc.narg(language), language),
    region = coalesce(sqlc.narg(region), region),
    poll_interval_minutes = coalesce(sqlc.narg(poll_interval_minutes), poll_interval_minutes),
    alert_enabled = coalesce(sqlc.narg(alert_enabled), alert_enabled),
    status = coalesce(sqlc.narg(status), status),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at;

-- user_notifications --------------------------------------------------------

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
