package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
)

// MonitorRepo implements monitor.Repository using PostgreSQL.
type MonitorRepo struct {
	db *sql.DB
}

// NewMonitorRepo creates a new Postgres-backed monitor repository.
func NewMonitorRepo(db *sql.DB) *MonitorRepo {
	return &MonitorRepo{db: db}
}

func (r *MonitorRepo) Create(ctx context.Context, userID int64, input monitor.CreateMonitorInput) (monitor.Monitor, error) {
	var m monitor.Monitor
	var configRaw []byte
	configJSON, _ := json.Marshal(map[string]interface{}{})
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO keyword_monitors (user_id, name, query_text, language, region, poll_interval_minutes, alert_enabled, alert_threshold_config)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at`,
		userID, input.Name, input.QueryText, input.Language, input.Region, input.PollIntervalMinutes, input.AlertEnabled, configJSON,
	).Scan(&m.ID, &m.UserID, &m.Name, &m.QueryText, &m.Language, &m.Region, &m.Status, &m.PollIntervalMinutes, &m.AlertEnabled, &configRaw, &m.LastPolledAt, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return m, err
	}
	if len(configRaw) > 0 {
		_ = json.Unmarshal(configRaw, &m.AlertThresholdConfig)
	}
	return m, nil
}

func (r *MonitorRepo) GetByID(ctx context.Context, id int64) (*monitor.Monitor, error) {
	var m monitor.Monitor
	var configRaw []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at
		 FROM keyword_monitors WHERE id = $1`, id,
	).Scan(&m.ID, &m.UserID, &m.Name, &m.QueryText, &m.Language, &m.Region, &m.Status, &m.PollIntervalMinutes, &m.AlertEnabled, &configRaw, &m.LastPolledAt, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(configRaw) > 0 {
		_ = json.Unmarshal(configRaw, &m.AlertThresholdConfig)
	}
	return &m, nil
}

func (r *MonitorRepo) ListByUser(ctx context.Context, userID int64) ([]monitor.Monitor, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at
		 FROM keyword_monitors WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []monitor.Monitor
	for rows.Next() {
		var m monitor.Monitor
		var configRaw []byte
		if err := rows.Scan(&m.ID, &m.UserID, &m.Name, &m.QueryText, &m.Language, &m.Region, &m.Status, &m.PollIntervalMinutes, &m.AlertEnabled, &configRaw, &m.LastPolledAt, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		if len(configRaw) > 0 {
			_ = json.Unmarshal(configRaw, &m.AlertThresholdConfig)
		}
		monitors = append(monitors, m)
	}
	return monitors, rows.Err()
}

func (r *MonitorRepo) Update(ctx context.Context, id int64, input monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	// Build dynamic SET clause.
	sets := []string{}
	args := []interface{}{}
	argIdx := 1

	if input.Name != nil {
		sets = append(sets, "name = $"+itoa(argIdx))
		args = append(args, *input.Name)
		argIdx++
	}
	if input.QueryText != nil {
		sets = append(sets, "query_text = $"+itoa(argIdx))
		args = append(args, *input.QueryText)
		argIdx++
	}
	if input.Language != nil {
		sets = append(sets, "language = $"+itoa(argIdx))
		args = append(args, *input.Language)
		argIdx++
	}
	if input.Region != nil {
		sets = append(sets, "region = $"+itoa(argIdx))
		args = append(args, *input.Region)
		argIdx++
	}
	if input.PollIntervalMinutes != nil {
		sets = append(sets, "poll_interval_minutes = $"+itoa(argIdx))
		args = append(args, *input.PollIntervalMinutes)
		argIdx++
	}
	if input.AlertEnabled != nil {
		sets = append(sets, "alert_enabled = $"+itoa(argIdx))
		args = append(args, *input.AlertEnabled)
		argIdx++
	}
	if input.Status != nil {
		sets = append(sets, "status = $"+itoa(argIdx))
		args = append(args, *input.Status)
		argIdx++
	}

	if len(sets) == 0 {
		m, err := r.GetByID(ctx, id)
		if err != nil {
			return monitor.Monitor{}, err
		}
		if m == nil {
			return monitor.Monitor{}, monitor.ErrNotFound
		}
		return *m, nil
	}

	sets = append(sets, "updated_at = now()")
	args = append(args, id)

	query := "UPDATE keyword_monitors SET "
	for i, s := range sets {
		if i > 0 {
			query += ", "
		}
		query += s
	}
	query += " WHERE id = $" + itoa(argIdx) + " RETURNING id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at"

	var m monitor.Monitor
	var configRaw []byte
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&m.ID, &m.UserID, &m.Name, &m.QueryText, &m.Language, &m.Region, &m.Status, &m.PollIntervalMinutes, &m.AlertEnabled, &configRaw, &m.LastPolledAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return monitor.Monitor{}, monitor.ErrNotFound
	}
	if err != nil {
		return m, err
	}
	if len(configRaw) > 0 {
		_ = json.Unmarshal(configRaw, &m.AlertThresholdConfig)
	}
	return m, nil
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
