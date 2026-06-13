package database

import (
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// TrendRepo implements trend.Repository using PostgreSQL.
type TrendRepo struct {
	db *sql.DB
}

// NewTrendRepo creates a new Postgres-backed trend repository.
func NewTrendRepo(db *sql.DB) *TrendRepo {
	return &TrendRepo{db: db}
}

// SaveTopicSnapshot inserts a topic snapshot record.
func (r *TrendRepo) SaveTopicSnapshot(snap trend.TopicSnapshot) error {
	_, err := r.db.Exec(
		`INSERT INTO topic_snapshots
			(topic_id, snapshot_time, post_count, unique_author_count, engagement_sum,
			 heat_score, trend_velocity)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		snap.TopicID, snap.SnapshotTime, snap.PostCount, snap.UniqueAuthorCount,
		snap.EngagementSum, snap.HeatScore, snap.TrendVelocity,
	)
	return err
}

// SaveMonitorSnapshot inserts a monitor snapshot record.
func (r *TrendRepo) SaveMonitorSnapshot(snap trend.MonitorSnapshot) error {
	_, err := r.db.Exec(
		`INSERT INTO monitor_snapshots
			(monitor_id, snapshot_time, new_post_count, active_topic_count,
			 total_engagement, top_topic_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		snap.MonitorID, snap.SnapshotTime, snap.NewPostCount,
		snap.ActiveTopicCount, snap.TotalEngagement, snap.TopTopicID,
	)
	return err
}

// GetPreviousTopicHeat returns the most recent heat score for a topic.
func (r *TrendRepo) GetPreviousTopicHeat(topicID int64) (float64, error) {
	var heat float64
	err := r.db.QueryRow(
		`SELECT heat_score FROM topic_snapshots
		 WHERE topic_id = $1 ORDER BY snapshot_time DESC LIMIT 1`, topicID,
	).Scan(&heat)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return heat, err
}
