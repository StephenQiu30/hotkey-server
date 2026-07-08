package repository

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"gorm.io/gorm"
)

// TopicQueryService implements topic.TopicQueryService using PostgreSQL via GORM.
type TopicQueryService struct {
	db *gorm.DB
}

// NewTopicQueryService creates a new Postgres-backed topic query service.
func NewTopicQueryService(db *gorm.DB) *TopicQueryService {
	return &TopicQueryService{db: db}
}

func (s *TopicQueryService) ListByMonitor(monitorID int64) ([]topic.TopicSummary, error) {
	rows, err := s.db.Raw(
		`SELECT t.id, t.title, t.summary, t.current_heat_score, t.trend_direction,
		        COUNT(tp.id) AS post_count
		 FROM topics t
		 LEFT JOIN topic_posts tp ON tp.topic_id = t.id
		 WHERE t.monitor_id = ? AND t.status = 'active'
		 GROUP BY t.id
		 ORDER BY t.current_heat_score DESC`, monitorID,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []topic.TopicSummary
	for rows.Next() {
		var ts topic.TopicSummary
		if err := rows.Scan(&ts.ID, &ts.Title, &ts.Summary, &ts.CurrentHeat, &ts.TrendDirection, &ts.PostCount); err != nil {
			return nil, err
		}
		summaries = append(summaries, ts)
	}
	return summaries, rows.Err()
}

func (s *TopicQueryService) GetMonitorID(ctx context.Context, topicID int64) (int64, error) {
	var monitorID int64
	err := s.db.WithContext(ctx).Raw(
		`SELECT monitor_id FROM topics WHERE id = ?`, topicID,
	).Scan(&monitorID).Error
	if err != nil {
		return 0, err
	}
	if monitorID == 0 {
		return 0, errors.New("topic not found")
	}
	return monitorID, nil
}
