package database

import (
	"encoding/json"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"gorm.io/gorm"
)

// ContentQueryService implements content.PostQueryService using PostgreSQL via GORM.
type ContentQueryService struct {
	db *gorm.DB
}

// NewContentQueryService creates a new Postgres-backed content query service.
func NewContentQueryService(db *gorm.DB) *ContentQueryService {
	return &ContentQueryService{db: db}
}

// ListPostsByMonitor returns posts matched to a monitor, ordered by score.
func (s *ContentQueryService) ListPostsByMonitor(monitorID int64, limit, offset int) ([]content.PostSummary, error) {
	rows, err := s.db.Raw(
		`SELECT pp.id, pp.platform_post_id, pp.author_name, pp.author_handle,
		        pp.content_text, pp.content_lang, pp.published_at,
		        pp.like_count, pp.reply_count, pp.repost_count, pp.quote_count, pp.view_count,
		        mph.heat_score, mph.relevance_score, mph.freshness_score, mph.final_score,
		        mph.matched_keywords
		 FROM monitor_post_hits mph
		 JOIN platform_posts pp ON pp.id = mph.post_id
		 WHERE mph.monitor_id = ?
		 ORDER BY mph.final_score DESC
		 LIMIT ? OFFSET ?`,
		monitorID, limit, offset,
	).Rows()
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
