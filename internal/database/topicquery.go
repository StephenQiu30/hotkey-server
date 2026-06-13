package database

import (
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// TopicQueryService implements topic.TopicQueryService using PostgreSQL.
type TopicQueryService struct {
	db *sql.DB
}

// NewTopicQueryService creates a new Postgres-backed topic query service.
func NewTopicQueryService(db *sql.DB) *TopicQueryService {
	return &TopicQueryService{db: db}
}

// ListByMonitor returns topic summaries for a given monitor.
func (s *TopicQueryService) ListByMonitor(monitorID int64) ([]topic.TopicSummary, error) {
	rows, err := s.db.Query(
		`SELECT t.id, t.title, t.summary, t.current_heat_score, t.trend_direction,
		        COUNT(tp.id) AS post_count
		 FROM topics t
		 LEFT JOIN topic_posts tp ON tp.topic_id = t.id
		 WHERE t.monitor_id = $1 AND t.status = 'active'
		 GROUP BY t.id
		 ORDER BY t.current_heat_score DESC`, monitorID)
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
