package database

import (
	"context"
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
)

// DigestQueryService implements digest.TopicFilter using PostgreSQL.
type DigestQueryService struct {
	db *sql.DB
}

// NewDigestQueryService creates a new Postgres-backed digest query service.
func NewDigestQueryService(db *sql.DB) *DigestQueryService {
	return &DigestQueryService{db: db}
}

// ListTopicsForDay returns active topics for a monitor that have at least one
// post with first_seen_at or published_at within the given window.
// Results are ordered by current_heat_score DESC.
func (s *DigestQueryService) ListTopicsForDay(monitorID int64, window digest.Window) ([]digest.TopicEntry, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT DISTINCT t.id, t.title, t.current_heat_score
		 FROM topics t
		 JOIN topic_posts tp ON tp.topic_id = t.id
		 JOIN monitor_post_hits mph ON mph.post_id = tp.post_id AND mph.monitor_id = t.monitor_id
		 JOIN platform_posts pp ON pp.id = tp.post_id
		 WHERE t.monitor_id = $1
		   AND t.status = 'active'
		   AND (mph.first_seen_at >= $2 AND mph.first_seen_at < $3
		        OR pp.published_at >= $2 AND pp.published_at < $3)
		 ORDER BY t.current_heat_score DESC`,
		monitorID, window.Start, window.End,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []digest.TopicEntry
	for rows.Next() {
		var t digest.TopicEntry
		if err := rows.Scan(&t.ID, &t.Title, &t.Heat); err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// FetchRepresentativePosts returns up to limit posts for a topic, ordered by
// membership_score DESC. Each post includes author name, content excerpt, and URL.
func (s *DigestQueryService) FetchRepresentativePosts(topicID int64, limit int) ([]digest.PostEntry, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT pp.id, pp.author_name, pp.content_text, pp.post_url, tp.membership_score
		 FROM topic_posts tp
		 JOIN platform_posts pp ON pp.id = tp.post_id
		 WHERE tp.topic_id = $1
		 ORDER BY tp.membership_score DESC
		 LIMIT $2`,
		topicID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []digest.PostEntry
	for rows.Next() {
		var p digest.PostEntry
		var contentText string
		if err := rows.Scan(&p.PostID, &p.AuthorName, &contentText, &p.PostURL, &p.MembershipScore); err != nil {
			return nil, err
		}
		// Truncate content to excerpt (first 200 chars)
		if len(contentText) > 200 {
			contentText = contentText[:200]
		}
		p.ContentExcerpt = contentText
		posts = append(posts, p)
	}
	return posts, rows.Err()
}
