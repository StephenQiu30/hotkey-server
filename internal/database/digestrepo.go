package database

import (
	"database/sql"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
)

// DigestRepo implements digest.Repository using PostgreSQL.
type DigestRepo struct {
	db *sql.DB
}

// NewDigestRepo creates a new Postgres-backed digest repository.
func NewDigestRepo(db *sql.DB) *DigestRepo {
	return &DigestRepo{db: db}
}

// Upsert inserts or updates a daily export record.
func (r *DigestRepo) Upsert(e digest.TopicDailyExport) (digest.TopicDailyExport, error) {
	var out digest.TopicDailyExport
	err := r.db.QueryRow(
		`INSERT INTO topic_daily_exports (monitor_id, topic_id, export_date, summary_text, markdown_path, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (monitor_id, topic_id, export_date) DO UPDATE SET
			 summary_text = EXCLUDED.summary_text,
			 markdown_path = EXCLUDED.markdown_path,
			 status = EXCLUDED.status,
			 error_message = '',
			 published_at = CASE WHEN EXCLUDED.status = 'published' THEN now() ELSE topic_daily_exports.published_at END
		 RETURNING id, monitor_id, topic_id, export_date, summary_text, markdown_path, status, error_message, published_at, created_at`,
		e.MonitorID, e.TopicID, e.ExportDate, e.SummaryText, e.MarkdownPath, string(e.Status),
	).Scan(&out.ID, &out.MonitorID, &out.TopicID, &out.ExportDate, &out.SummaryText,
		&out.MarkdownPath, &out.Status, &out.ErrorMessage, &out.PublishedAt, &out.CreatedAt)
	return out, err
}

// GetByTopicDate retrieves a daily export by topic_id and export_date.
func (r *DigestRepo) GetByTopicDate(topicID int64, exportDate time.Time) (*digest.TopicDailyExport, error) {
	var e digest.TopicDailyExport
	err := r.db.QueryRow(
		`SELECT id, monitor_id, topic_id, export_date, summary_text, markdown_path, status, error_message, published_at, created_at
		 FROM topic_daily_exports
		 WHERE topic_id = $1 AND export_date = $2`,
		topicID, exportDate,
	).Scan(&e.ID, &e.MonitorID, &e.TopicID, &e.ExportDate, &e.SummaryText,
		&e.MarkdownPath, &e.Status, &e.ErrorMessage, &e.PublishedAt, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
