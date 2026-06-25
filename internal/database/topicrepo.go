package database

import (
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"gorm.io/gorm"
)

// TopicRepo implements topic.Repository using PostgreSQL via GORM.
type TopicRepo struct {
	db *gorm.DB
}

// NewTopicRepo creates a new Postgres-backed topic repository.
func NewTopicRepo(db *gorm.DB) *TopicRepo {
	return &TopicRepo{db: db}
}

func (r *TopicRepo) UpsertTopic(monitorID int64, t topic.Topic) (int64, error) {
	var id int64
	err := r.db.Raw(
		`INSERT INTO topics (monitor_id, topic_key, title)
		 VALUES (?, ?, ?)
		 ON CONFLICT (monitor_id, topic_key) DO UPDATE SET
			 title = EXCLUDED.title,
			 last_active_at = now(),
			 updated_at = now()
		 RETURNING id`,
		monitorID, t.TopicKey, t.Title,
	).Scan(&id).Error
	return id, err
}

func (r *TopicRepo) AddPostToTopic(topicID, postID int64, membershipScore float64) error {
	return r.db.Exec(
		`INSERT INTO topic_posts (topic_id, post_id, membership_score)
		 VALUES (?, ?, ?)
		 ON CONFLICT (topic_id, post_id) DO UPDATE SET
			 membership_score = EXCLUDED.membership_score`,
		topicID, postID, membershipScore,
	).Error
}

func (r *TopicRepo) ListByMonitor(monitorID int64) ([]topic.TopicSummary, error) {
	rows, err := r.db.Raw(
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
		var s topic.TopicSummary
		if err := rows.Scan(&s.ID, &s.Title, &s.Summary, &s.CurrentHeat, &s.TrendDirection, &s.PostCount); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}
