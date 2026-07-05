package database

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"gorm.io/gorm"
)

// DigestQueryService implements digest.TopicFilter and digest.EventFilter
// using PostgreSQL via GORM.
type DigestQueryService struct {
	db *gorm.DB
}

// NewDigestQueryService creates a new Postgres-backed digest query service.
func NewDigestQueryService(db *gorm.DB) *DigestQueryService {
	return &DigestQueryService{db: db}
}

func (s *DigestQueryService) ListTopicsForDay(ctx context.Context, monitorID int64, window digest.Window) ([]digest.TopicEntry, error) {
	rows, err := s.db.WithContext(ctx).Raw(
		`SELECT DISTINCT t.id, t.title, t.current_heat_score
		 FROM topics t
		 JOIN topic_posts tp ON tp.topic_id = t.id
		 JOIN monitor_post_hits mph ON mph.post_id = tp.post_id AND mph.monitor_id = t.monitor_id
		 JOIN platform_posts pp ON pp.id = tp.post_id
		 WHERE t.monitor_id = ?
		   AND t.status = 'active'
		   AND ((mph.first_seen_at >= ? AND mph.first_seen_at < ?)
		        OR (pp.published_at >= ? AND pp.published_at < ?))
		 ORDER BY t.current_heat_score DESC`,
		monitorID, window.Start, window.End, window.Start, window.End,
	).Rows()
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

func (s *DigestQueryService) FetchRepresentativePosts(ctx context.Context, topicID int64, limit int) ([]digest.PostEntry, error) {
	rows, err := s.db.WithContext(ctx).Raw(
		`SELECT pp.id, pp.author_name, pp.content_text, pp.post_url, tp.membership_score
		 FROM topic_posts tp
		 JOIN platform_posts pp ON pp.id = tp.post_id
		 WHERE tp.topic_id = ?
		 ORDER BY tp.membership_score DESC
		 LIMIT ?`,
		topicID, limit,
	).Rows()
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
		runes := []rune(contentText)
		if len(runes) > 200 {
			runes = runes[:200]
		}
		p.ContentExcerpt = string(runes)
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (s *DigestQueryService) ListEventsForDay(ctx context.Context, window digest.Window, topN int) ([]digest.EventEntry, error) {
	rows, err := s.db.WithContext(ctx).Raw(
		`SELECT id, name, heat_score, platform, summary
		 FROM hot_events
		 WHERE status = 'active'
		   AND last_seen_at >= ? AND last_seen_at < ?
		 ORDER BY heat_score DESC
		 LIMIT ?`,
		window.Start, window.End, topN,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []digest.EventEntry
	for rows.Next() {
		var e digest.EventEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.HeatScore, &e.Platform, &e.Summary); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
