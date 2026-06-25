package database

import (
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"gorm.io/gorm"
)

// JobQueryRepo implements worker read queries for topic aggregation and snapshots.
type JobQueryRepo struct {
	db *gorm.DB
}

func NewJobQueryRepo(db *gorm.DB) *JobQueryRepo {
	return &JobQueryRepo{db: db}
}

func (r *JobQueryRepo) GetUnclusteredPosts(monitorID int64) ([]jobs.PostCandidate, error) {
	rows, err := r.db.Raw(
		`SELECT pp.id, pp.content_text
		 FROM monitor_post_hits mph
		 JOIN platform_posts pp ON pp.id = mph.post_id
		 WHERE mph.monitor_id = ?
		   AND NOT EXISTS (
		     SELECT 1 FROM topic_posts tp WHERE tp.post_id = pp.id
		   )
		 ORDER BY mph.final_score DESC
		 LIMIT 100`, monitorID,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []jobs.PostCandidate
	for rows.Next() {
		var c jobs.PostCandidate
		if err := rows.Scan(&c.PostID, &c.Text); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func (r *JobQueryRepo) GetTopicDataForMonitor(monitorID int64) ([]jobs.TopicData, error) {
	rows, err := r.db.Raw(
		`SELECT t.id,
		        COUNT(tp.id) AS post_count,
		        0 AS unique_author_count,
		        0 AS engagement_sum,
		        COALESCE(t.current_heat_score, 0) AS heat_score,
		        COALESCE(ts_prev.heat_score, 0) AS previous_heat
		 FROM topics t
		 LEFT JOIN topic_posts tp ON tp.topic_id = t.id
		 LEFT JOIN LATERAL (
		     SELECT heat_score FROM topic_snapshots
		     WHERE topic_id = t.id ORDER BY snapshot_time DESC LIMIT 1
		 ) ts_prev ON true
		 WHERE t.monitor_id = ? AND t.status = 'active'
		 GROUP BY t.id, t.current_heat_score, ts_prev.heat_score`, monitorID,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []jobs.TopicData
	for rows.Next() {
		var d jobs.TopicData
		if err := rows.Scan(&d.TopicID, &d.PostCount, &d.UniqueAuthorCount, &d.EngagementSum, &d.HeatScore, &d.PreviousHeat); err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, rows.Err()
}
