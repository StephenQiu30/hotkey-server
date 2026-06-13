package database

import (
	"database/sql"
	"encoding/json"

	"github.com/StephenQiu30/hotkey-server/internal/content"
)

// ContentQueryService implements content.PostQueryService using PostgreSQL.
type ContentQueryService struct {
	db *sql.DB
}

// NewContentQueryService creates a new Postgres-backed content query service.
func NewContentQueryService(db *sql.DB) *ContentQueryService {
	return &ContentQueryService{db: db}
}

// ListPostsByMonitor returns posts matched to a monitor, ordered by score.
func (s *ContentQueryService) ListPostsByMonitor(monitorID int64, limit, offset int) ([]content.PostSummary, error) {
	rows, err := s.db.Query(
		`SELECT pp.id, pp.platform_post_id, pp.author_name, pp.author_handle,
		        pp.content_text, pp.content_lang, pp.published_at,
		        pp.like_count, pp.reply_count, pp.repost_count, pp.quote_count, pp.view_count,
		        mph.heat_score, mph.relevance_score, mph.freshness_score, mph.final_score,
		        mph.matched_keywords
		 FROM monitor_post_hits mph
		 JOIN platform_posts pp ON pp.id = mph.post_id
		 WHERE mph.monitor_id = $1
		 ORDER BY mph.final_score DESC
		 LIMIT $2 OFFSET $3`,
		monitorID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []content.PostSummary
	for rows.Next() {
		var p content.PostSummary
		var kwJSON []byte
		if err := rows.Scan(
			&p.ID, &p.PlatformPostID, &p.AuthorName, &p.AuthorHandle,
			&p.ContentText, &p.ContentLang, &p.PublishedAt,
			&p.LikeCount, &p.ReplyCount, &p.RepostCount, &p.QuoteCount, &p.ViewCount,
			&p.HeatScore, &p.RelevanceScore, &p.FreshnessScore, &p.FinalScore,
			&kwJSON,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(kwJSON, &p.MatchedKeywords)
		posts = append(posts, p)
	}
	return posts, rows.Err()
}
