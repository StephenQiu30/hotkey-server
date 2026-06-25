package database

import (
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"gorm.io/gorm"
)

// DigestRepo implements digest.Repository using PostgreSQL via GORM.
type DigestRepo struct {
	db *gorm.DB
}

// NewDigestRepo creates a new Postgres-backed digest repository.
func NewDigestRepo(db *gorm.DB) *DigestRepo {
	return &DigestRepo{db: db}
}

func (r *DigestRepo) Upsert(e digest.TopicDailyExport) (digest.TopicDailyExport, error) {
	var out digest.TopicDailyExport
	err := r.db.Raw(
		`INSERT INTO topic_daily_exports (monitor_id, topic_id, export_date, summary_text, markdown_path, status)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (monitor_id, topic_id, export_date) DO UPDATE SET
			 summary_text = EXCLUDED.summary_text,
			 markdown_path = EXCLUDED.markdown_path,
			 status = EXCLUDED.status,
			 error_message = '',
			 published_at = CASE WHEN EXCLUDED.status = 'published' THEN now() ELSE topic_daily_exports.published_at END
		 RETURNING id, monitor_id, topic_id, export_date, summary_text, markdown_path, status, error_message, published_at, created_at`,
		e.MonitorID, e.TopicID, e.ExportDate, e.SummaryText, e.MarkdownPath, string(e.Status),
	).Scan(&out).Error
	return out, err
}

func (r *DigestRepo) GetByTopicDate(topicID int64, exportDate time.Time) (*digest.TopicDailyExport, error) {
	var e digest.TopicDailyExport
	err := r.db.Raw(
		`SELECT id, monitor_id, topic_id, export_date, summary_text, markdown_path, status, error_message, published_at, created_at
		 FROM topic_daily_exports
		 WHERE topic_id = ? AND export_date = ?`,
		topicID, exportDate,
	).Scan(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && e.ID == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
