package database

import (
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"gorm.io/gorm"
)

// TrendRepo implements trend.Repository using PostgreSQL via GORM.
type TrendRepo struct {
	db *gorm.DB
}

// NewTrendRepo creates a new Postgres-backed trend repository.
func NewTrendRepo(db *gorm.DB) *TrendRepo {
	return &TrendRepo{db: db}
}

func (r *TrendRepo) SaveTopicSnapshot(snap trend.TopicSnapshot) error {
	return r.db.Exec(
		`INSERT INTO topic_snapshots
			(topic_id, snapshot_time, post_count, unique_author_count, engagement_sum,
			 heat_score, trend_velocity)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (topic_id, snapshot_time) DO UPDATE SET
		   post_count = EXCLUDED.post_count,
		   unique_author_count = EXCLUDED.unique_author_count,
		   engagement_sum = EXCLUDED.engagement_sum,
		   heat_score = EXCLUDED.heat_score,
		   trend_velocity = EXCLUDED.trend_velocity`,
		snap.TopicID, snap.SnapshotTime, snap.PostCount, snap.UniqueAuthorCount,
		snap.EngagementSum, snap.HeatScore, snap.TrendVelocity,
	).Error
}

func (r *TrendRepo) SaveMonitorSnapshot(snap trend.MonitorSnapshot) error {
	return r.db.Exec(
		`INSERT INTO monitor_snapshots
			(monitor_id, snapshot_time, new_post_count, active_topic_count,
			 total_engagement, top_topic_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (monitor_id, snapshot_time) DO UPDATE SET
		   new_post_count = EXCLUDED.new_post_count,
		   active_topic_count = EXCLUDED.active_topic_count,
		   total_engagement = EXCLUDED.total_engagement,
		   top_topic_id = EXCLUDED.top_topic_id`,
		snap.MonitorID, snap.SnapshotTime, snap.NewPostCount,
		snap.ActiveTopicCount, snap.TotalEngagement, snap.TopTopicID,
	).Error
}

func (r *TrendRepo) GetPreviousTopicHeat(topicID int64) (float64, error) {
	var heat float64
	err := r.db.Raw(
		`SELECT heat_score FROM topic_snapshots
		 WHERE topic_id = ? ORDER BY snapshot_time DESC LIMIT 1`, topicID,
	).Scan(&heat).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return heat, err
}
