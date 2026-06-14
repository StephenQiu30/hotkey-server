package database

import (
	"context"
	"database/sql"
)

// DigestExportRepo manages topic_daily_exports persistence.
type DigestExportRepo struct {
	db *sql.DB
}

// NewDigestExportRepo creates a new DigestExportRepo.
func NewDigestExportRepo(db *sql.DB) *DigestExportRepo {
	return &DigestExportRepo{db: db}
}

// ExportRecord represents a row in topic_daily_exports.
type ExportRecord struct {
	ID           int64
	MonitorID    int64
	TopicID      int64
	ExportDate   string
	SummaryText  string
	MarkdownPath string
	Status       string
	ErrorMessage string
}

// UpsertExport inserts or updates an export record.
func (r *DigestExportRepo) UpsertExport(ctx context.Context, rec ExportRecord) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO topic_daily_exports
			(monitor_id, topic_id, export_date, summary_text, markdown_path, status, error_message)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (monitor_id, topic_id, export_date) DO UPDATE SET
			 summary_text = EXCLUDED.summary_text,
			 markdown_path = EXCLUDED.markdown_path,
			 status = EXCLUDED.status,
			 error_message = EXCLUDED.error_message
		 RETURNING id`,
		rec.MonitorID, rec.TopicID, rec.ExportDate,
		rec.SummaryText, rec.MarkdownPath, rec.Status, rec.ErrorMessage,
	).Scan(&id)
	return id, err
}

// GetLastRunDate returns the last run date from the metadata table.
func (r *DigestExportRepo) GetLastRunDate(ctx context.Context) (string, error) {
	var date string
	err := r.db.QueryRowContext(ctx,
		`SELECT value FROM job_metadata WHERE key = 'daily_publish_last_run'`,
	).Scan(&date)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return date, err
}

// SetLastRunDate persists the last run date.
func (r *DigestExportRepo) SetLastRunDate(ctx context.Context, date string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO job_metadata (key, value)
		 VALUES ('daily_publish_last_run', $1)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		date,
	)
	return err
}
