package gormimpl

import (
	"context"

	"gorm.io/gorm"
)

// WorkerRepo provides the aggregated queries used by background workers.
type WorkerRepo struct {
	db *gorm.DB
}

func NewWorkerRepo(db *gorm.DB) *WorkerRepo {
	return &WorkerRepo{db: db}
}

// QueryTopicStats retrieves topic stats for snapshot building.
// Uses Raw+Scan (Bug #8 fix: was Model+Select aliasing).
func (r *WorkerRepo) QueryTopicStats(ctx context.Context, monitorID int64) ([]struct {
	TopicID   int64
	PostCount int
	HeatSum   float64
}, error) {
	sql := `
		SELECT t.id AS topic_id,
		       COUNT(tp.id) AS post_count,
		       COALESCE(AVG(tp.membership_score), 0) AS heat_sum
		FROM topics t
		LEFT JOIN topic_posts tp ON tp.topic_id = t.id
		WHERE t.monitor_id = ? AND t.status = 'active'
		GROUP BY t.id
	`
	var results []struct {
		TopicID   int64
		PostCount int
		HeatSum   float64
	}
	if err := r.db.WithContext(ctx).Raw(sql, monitorID).Scan(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

// QueryEngagementStats retrieves total engagement for a monitor.
func (r *WorkerRepo) QueryEngagementStats(ctx context.Context, monitorID int64) (int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(pp.like_count + pp.reply_count + pp.repost_count + pp.view_count), 0)
		FROM platform_posts pp
		JOIN monitor_post_hits mph ON mph.post_id = pp.id
		WHERE mph.monitor_id = ?
	`, monitorID).Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// QueryNewPostCount returns the count of posts since the given time.
func (r *WorkerRepo) QueryNewPostCount(ctx context.Context, monitorID int64, since string) (int, error) {
	var count int
	if err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM platform_posts pp
		JOIN monitor_post_hits mph ON mph.post_id = pp.id
		WHERE mph.monitor_id = ? AND pp.published_at >= ?
	`, monitorID, since).Scan(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
