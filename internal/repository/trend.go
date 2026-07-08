package repository

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/service"
	"gorm.io/gorm"
)

// TrendQueryService implements service.TrendQueryService using PostgreSQL via GORM.
type TrendQueryService struct {
	db *gorm.DB
}

// NewTrendQueryService creates a new Postgres-backed trend query service.
func NewTrendQueryService(db *gorm.DB) *TrendQueryService {
	return &TrendQueryService{db: db}
}

func (s *TrendQueryService) GetTopicTrends(topicID int64, since time.Time) ([]service.TrendPoint, error) {
	rows, err := s.db.Raw(
		`SELECT snapshot_time, heat_score, trend_velocity,
		        CASE WHEN trend_velocity > 0.05 THEN 'rising'
		             WHEN trend_velocity < -0.05 THEN 'falling'
		             ELSE 'flat' END AS trend_direction
		 FROM topic_snapshots
		 WHERE topic_id = ? AND snapshot_time >= ?
		 ORDER BY snapshot_time ASC`,
		topicID, since,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []service.TrendPoint
	for rows.Next() {
		var p service.TrendPoint
		if err := rows.Scan(&p.Time, &p.HeatScore, &p.TrendVelocity, &p.TrendDirection); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

func (s *TrendQueryService) GetMonitorTrends(monitorID int64, since time.Time) ([]service.TrendPoint, error) {
	rows, err := s.db.Raw(
		`SELECT snapshot_time, 0 AS heat_score, 0 AS trend_velocity, 'flat' AS trend_direction
		 FROM monitor_snapshots
		 WHERE monitor_id = ? AND snapshot_time >= ?
		 ORDER BY snapshot_time ASC`,
		monitorID, since,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []service.TrendPoint
	for rows.Next() {
		var p service.TrendPoint
		if err := rows.Scan(&p.Time, &p.HeatScore, &p.TrendVelocity, &p.TrendDirection); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}
