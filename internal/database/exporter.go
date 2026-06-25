package database

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"gorm.io/gorm"
)

// Exporter implements jobs.TopicExporter using PostgreSQL via GORM.
type Exporter struct {
	db *gorm.DB
}

// NewExporter creates a new Postgres-backed topic exporter.
func NewExporter(db *gorm.DB) *Exporter {
	return &Exporter{db: db}
}

func (e *Exporter) IsExported(ctx context.Context, topicID int64, date string) (bool, error) {
	exportDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false, err
	}

	var status string
	err = e.db.WithContext(ctx).Raw(
		`SELECT status FROM topic_daily_exports WHERE topic_id = ? AND export_date = ?`,
		topicID, exportDate,
	).Scan(&status).Error

	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && status == "") {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return status == string(digest.StatusPublished), nil
}

func (e *Exporter) MarkExported(ctx context.Context, topicID int64, date string) error {
	exportDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}

	return e.db.WithContext(ctx).Exec(
		`UPDATE topic_daily_exports SET status = ?, published_at = now() WHERE topic_id = ? AND export_date = ?`,
		string(digest.StatusPublished), topicID, exportDate,
	).Error
}

func (e *Exporter) MarkFailed(ctx context.Context, topicID int64, date string, reason string) error {
	exportDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}

	return e.db.WithContext(ctx).Exec(
		`UPDATE topic_daily_exports SET status = ?, error_message = ? WHERE topic_id = ? AND export_date = ?`,
		string(digest.StatusFailed), reason, topicID, exportDate,
	).Error
}
