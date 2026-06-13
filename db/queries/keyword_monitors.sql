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
