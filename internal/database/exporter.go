package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
)

// Exporter implements jobs.TopicExporter using PostgreSQL.
type Exporter struct {
	db *sql.DB
}

// NewExporter creates a new Postgres-backed topic exporter.
func NewExporter(db *sql.DB) *Exporter {
	return &Exporter{db: db}
}

// IsExported reports whether the topic+date combination has already been exported.
func (e *Exporter) IsExported(ctx context.Context, topicID int64, date string) (bool, error) {
	exportDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false, err
	}

	var status string
	err = e.db.QueryRowContext(ctx,
		`SELECT status FROM topic_daily_exports WHERE topic_id = $1 AND export_date = $2`,
		topicID, exportDate,
	).Scan(&status)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return status == string(digest.StatusPublished), nil
}

// MarkExported records a successful export.
func (e *Exporter) MarkExported(ctx context.Context, topicID int64, date string) error {
	exportDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}

	_, err = e.db.ExecContext(ctx,
		`UPDATE topic_daily_exports SET status = $1, published_at = now() WHERE topic_id = $2 AND export_date = $3`,
		string(digest.StatusPublished), topicID, exportDate,
	)
	return err
}

// MarkFailed records a failed export.
func (e *Exporter) MarkFailed(ctx context.Context, topicID int64, date string, reason string) error {
	exportDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}

	_, err = e.db.ExecContext(ctx,
		`UPDATE topic_daily_exports SET status = $1, error_message = $2 WHERE topic_id = $3 AND export_date = $4`,
		string(digest.StatusFailed), reason, topicID, exportDate,
	)
	return err
}
