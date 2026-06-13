package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/StephenQiu30/hotkey-server/internal/content"
)

// ContentRepo implements content.PostRepository and content.HitRepository
// using PostgreSQL.
type ContentRepo struct {
	db *sql.DB
}

// NewContentRepo creates a new Postgres-backed content repository.
func NewContentRepo(db *sql.DB) *ContentRepo {
	return &ContentRepo{db: db}
}

// UpsertPost inserts or updates a normalized post and returns its ID.
func (r *ContentRepo) UpsertPost(ctx context.Context, post content.NormalizedPost) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO platform_posts
			(platform, platform_post_id, author_platform_id, author_name, author_handle,
			 content_text, content_lang, post_url, published_at,
			 like_count, reply_count, repost_count, quote_count, view_count, normalized_hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT (platform, platform_post_id) DO UPDATE SET
			 author_name = EXCLUDED.author_name,
			 author_handle = EXCLUDED.author_handle,
			 content_text = EXCLUDED.content_text,
			 like_count = EXCLUDED.like_count,
			 reply_count = EXCLUDED.reply_count,
			 repost_count = EXCLUDED.repost_count,
			 quote_count = EXCLUDED.quote_count,
			 view_count = EXCLUDED.view_count,
			 updated_at = now()
		 RETURNING id`,
		post.Platform, post.PlatformPostID, post.AuthorPlatformID,
		post.AuthorName, post.AuthorHandle, post.ContentText, post.ContentLang,
		post.PostURL, post.PublishedAt, post.LikeCount, post.ReplyCount,
		post.RepostCount, post.QuoteCount, post.ViewCount, post.NormalizedHash,
	).Scan(&id)
	return id, err
}

// GetPostByPlatformID retrieves a post by its platform and platform post ID.
func (r *ContentRepo) GetPostByPlatformID(ctx context.Context, platform, platformPostID string) (*content.NormalizedPost, error) {
	var p content.NormalizedPost
	err := r.db.QueryRowContext(ctx,
		`SELECT platform, platform_post_id, author_platform_id, author_name, author_handle,
		        content_text, content_lang, post_url, published_at,
		        like_count, reply_count, repost_count, quote_count, view_count, normalized_hash
		 FROM platform_posts WHERE platform = $1 AND platform_post_id = $2`,
		platform, platformPostID,
	).Scan(&p.Platform, &p.PlatformPostID, &p.AuthorPlatformID,
		&p.AuthorName, &p.AuthorHandle, &p.ContentText, &p.ContentLang,
		&p.PostURL, &p.PublishedAt, &p.LikeCount, &p.ReplyCount,
		&p.RepostCount, &p.QuoteCount, &p.ViewCount, &p.NormalizedHash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpsertHit inserts or updates a monitor-post hit relationship.
func (r *ContentRepo) UpsertHit(ctx context.Context, hit content.MonitorHit) error {
	kwJSON, _ := json.Marshal(hit.MatchedKeywords)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO monitor_post_hits (monitor_id, post_id, matched_keywords, relevance_score)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (monitor_id, post_id) DO UPDATE SET
			 matched_keywords = EXCLUDED.matched_keywords,
			 relevance_score = EXCLUDED.relevance_score,
			 last_seen_at = now()`,
		hit.MonitorID, hit.PostID, kwJSON, hit.RelevanceScore,
	)
	return err
}

// GetHitsByMonitor retrieves all hits for a given monitor.
func (r *ContentRepo) GetHitsByMonitor(ctx context.Context, monitorID int64) ([]content.MonitorHit, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT monitor_id, post_id, matched_keywords, relevance_score
		 FROM monitor_post_hits WHERE monitor_id = $1 ORDER BY first_seen_at DESC`, monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []content.MonitorHit
	for rows.Next() {
		var h content.MonitorHit
		var kwJSON []byte
		if err := rows.Scan(&h.MonitorID, &h.PostID, &kwJSON, &h.RelevanceScore); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(kwJSON, &h.MatchedKeywords)
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
