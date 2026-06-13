package database

import (
	"database/sql"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// TrendQueryService implements trend.TrendQueryService using PostgreSQL.
type TrendQueryService struct {
	db *sql.DB
}

// NewTrendQueryService creates a new Postgres-backed trend query service.
func NewTrendQueryService(db *sql.DB) *TrendQueryService {
	return &TrendQueryService{db: db}
}

// GetTopicTrends returns trend data points for a topic since the given time.
func (s *TrendQueryService) GetTopicTrends(topicID int64, since time.Time) ([]trend.TrendPoint, error) {
	rows, err := s.db.Query(
		`SELECT snapshot_time, heat_score, trend_velocity,
		        CASE WHEN trend_velocity > 0.05 THEN 'rising'
		             WHEN trend_velocity < -0.05 THEN 'falling'
		             ELSE 'flat' END AS trend_direction
		 FROM topic_snapshots
		 WHERE topic_id = $1 AND snapshot_time >= $2
		 ORDER BY snapshot_time ASC`,
		topicID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []trend.TrendPoint
	for rows.Next() {
		var p trend.TrendPoint
		if err := rows.Scan(&p.Time, &p.HeatScore, &p.TrendVelocity, &p.TrendDirection); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetMonitorTrends returns trend data points for a monitor since the given time.
func (s *TrendQueryService) GetMonitorTrends(monitorID int64, since time.Time) ([]trend.TrendPoint, error) {
	rows, err := s.db.Query(
		`SELECT snapshot_time, 0 AS heat_score, 0 AS trend_velocity, 'flat' AS trend_direction
		 FROM monitor_snapshots
		 WHERE monitor_id = $1 AND snapshot_time >= $2
		 ORDER BY snapshot_time ASC`,
		monitorID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []trend.TrendPoint
	for rows.Next() {
		var p trend.TrendPoint
		if err := rows.Scan(&p.Time, &p.HeatScore, &p.TrendVelocity, &p.TrendDirection); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}
