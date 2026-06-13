package database

import (
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// TopicRepo implements topic.Repository using PostgreSQL.
type TopicRepo struct {
	db *sql.DB
}

// NewTopicRepo creates a new Postgres-backed topic repository.
func NewTopicRepo(db *sql.DB) *TopicRepo {
	return &TopicRepo{db: db}
}

// UpsertTopic inserts or updates a topic and returns its ID.
func (r *TopicRepo) UpsertTopic(monitorID int64, t topic.Topic) (int64, error) {
	var id int64
	err := r.db.QueryRow(
		`INSERT INTO topics (monitor_id, topic_key, title)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (monitor_id, topic_key) DO UPDATE SET
			 title = EXCLUDED.title,
			 last_active_at = now(),
			 updated_at = now()
		 RETURNING id`,
		monitorID, t.TopicKey, t.Title,
	).Scan(&id)
	return id, err
}

// AddPostToTopic adds a post to a topic with the given membership score.
func (r *TopicRepo) AddPostToTopic(topicID, postID int64, membershipScore float64) error {
	_, err := r.db.Exec(
		`INSERT INTO topic_posts (topic_id, post_id, membership_score)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (topic_id, post_id) DO UPDATE SET
			 membership_score = EXCLUDED.membership_score`,
		topicID, postID, membershipScore,
	)
	return err
}

// ListByMonitor returns topic summaries for a given monitor.
func (r *TopicRepo) ListByMonitor(monitorID int64) ([]topic.TopicSummary, error) {
	rows, err := r.db.Query(
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
		var s topic.TopicSummary
		if err := rows.Scan(&s.ID, &s.Title, &s.Summary, &s.CurrentHeat, &s.TrendDirection, &s.PostCount); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}
