package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"gorm.io/gorm"
)

type MonitorRepo struct {
	db *gorm.DB
}

func NewMonitorRepo(db *gorm.DB) *MonitorRepo {
	return &MonitorRepo{db: db}
}

func (r *MonitorRepo) Create(ctx context.Context, userID int64, input monitor.CreateMonitorInput) (monitor.Monitor, error) {
	var m monitor.Monitor
	var configRaw []byte
	configJSON, _ := json.Marshal(map[string]interface{}{})
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO keyword_monitors (user_id, name, query_text, language, region, poll_interval_minutes, alert_enabled, alert_threshold_config)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at`,
		userID, input.Name, input.QueryText, input.Language, input.Region, input.PollIntervalMinutes, input.AlertEnabled, configJSON,
	).Row().Scan(&m.ID, &m.UserID, &m.Name, &m.QueryText, &m.Language, &m.Region, &m.Status, &m.PollIntervalMinutes, &m.AlertEnabled, &configRaw, &m.LastPolledAt, &m.CreatedAt, &m.UpdatedAt)
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
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at
		 FROM keyword_monitors WHERE id = ?`, id,
	).Row().Scan(&m.ID, &m.UserID, &m.Name, &m.QueryText, &m.Language, &m.Region, &m.Status, &m.PollIntervalMinutes, &m.AlertEnabled, &configRaw, &m.LastPolledAt, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
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
	rows, err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, name, query_text, language, region, status, poll_interval_minutes, alert_enabled, alert_threshold_config, last_polled_at, created_at, updated_at
		 FROM keyword_monitors WHERE user_id = ? ORDER BY created_at DESC`, userID,
	).Rows()
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

// ListActiveIDs returns IDs of all active monitors.
func (r *MonitorRepo) ListActiveIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.WithContext(ctx).Raw(
		`SELECT id FROM keyword_monitors WHERE status = 'active' ORDER BY id`,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *MonitorRepo) Update(ctx context.Context, id int64, input monitor.UpdateMonitorInput) (monitor.Monitor, error) {
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
	err := r.db.WithContext(ctx).Raw(query, args...).Row().Scan(
		&m.ID, &m.UserID, &m.Name, &m.QueryText, &m.Language, &m.Region, &m.Status, &m.PollIntervalMinutes, &m.AlertEnabled, &configRaw, &m.LastPolledAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
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
