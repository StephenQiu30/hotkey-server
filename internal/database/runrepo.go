package database

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"gorm.io/gorm"
)

// RunRepo implements jobs.RunRepository via GORM.
type RunRepo struct {
	db *gorm.DB
}

func NewRunRepo(db *gorm.DB) *RunRepo {
	return &RunRepo{db: db}
}

func (r *RunRepo) CreateRun(ctx context.Context, run jobs.MonitorRun) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO monitor_runs (monitor_id, platform, run_type, status, started_at, fetched_count, stored_count, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		run.MonitorID, run.Platform, run.RunType, run.Status, run.StartedAt,
		run.FetchedCount, run.StoredCount, run.ErrorMessage,
	).Scan(&id).Error
	return id, err
}

func (r *RunRepo) UpdateRun(ctx context.Context, runID int64, run jobs.MonitorRun) error {
	return r.db.WithContext(ctx).Exec(
		`UPDATE monitor_runs SET status = ?, finished_at = ?, fetched_count = ?,
		 stored_count = ?, error_message = ? WHERE id = ?`,
		run.Status, run.FinishedAt, run.FetchedCount, run.StoredCount, run.ErrorMessage, runID,
	).Error
}
