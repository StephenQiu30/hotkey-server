package gormimpl

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ExporterRepo provides bulk export operations.
type ExporterRepo struct {
	db *gorm.DB
}

func NewExporterRepo(db *gorm.DB) *ExporterRepo {
	return &ExporterRepo{db: db}
}

// QueryPostsByMonitor retrieves platform_posts joined with hits for a monitor.
// Uses Raw+Scan instead of Model+alias (Bug #8 fix).
func (r *ExporterRepo) QueryPostsByMonitor(ctx context.Context, monitorID int64, since time.Time) ([]map[string]any, error) {
	var results []map[string]any
	rows, err := r.db.WithContext(ctx).Raw(`
		SELECT pp.id, pp.platform, pp.platform_post_id, pp.author_name,
		       pp.content_text, pp.published_at, pp.like_count, pp.reply_count,
		       pp.repost_count, pp.view_count,
		       mph.heat_score, mph.relevance_score, mph.final_score
		FROM platform_posts pp
		JOIN monitor_post_hits mph ON mph.post_id = pp.id
		WHERE mph.monitor_id = ? AND pp.published_at >= ?
		ORDER BY pp.published_at DESC
	`, monitorID, since).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	for rows.Next() {
		row := make(map[string]any)
		values := make([]any, len(cols))
		for i := range cols {
			values[i] = new(any)
		}
		if err := rows.Scan(values...); err != nil {
			return nil, err
		}
		for i, col := range cols {
			row[col] = *(values[i].(*any))
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// CreateExportBundle inserts a new export_bundle record.
func (r *ExporterRepo) CreateExportBundle(ctx context.Context, monitorID int64, bundleKey, bundleKind string, dateStart, dateEnd *time.Time) (int64, error) {
	m := ExportBundle{
		MonitorID:  monitorID,
		BundleKey:  bundleKey,
		BundleKind: bundleKind,
		DateStart:  dateStart,
		DateEnd:    dateEnd,
		Status:     "pending",
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return 0, err
	}
	return m.ID, nil
}

// UpdateBundleStatus updates the status of an export bundle.
func (r *ExporterRepo) UpdateBundleStatus(ctx context.Context, id int64, status string) error {
	return r.db.WithContext(ctx).Model(&ExportBundle{}).
		Where("id = ?", id).
		Update("status", status).Error
}
