package database

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"gorm.io/gorm"
)

// ContentRepo implements content.PostRepository and content.HitRepository via GORM.
type ContentRepo struct {
	db *gorm.DB
}

// NewContentRepo creates a new Postgres-backed content repository.
func NewContentRepo(db *gorm.DB) *ContentRepo {
	return &ContentRepo{db: db}
}

func (r *ContentRepo) UpsertPost(ctx context.Context, post content.NormalizedPost) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO platform_posts
			(platform, platform_post_id, author_platform_id, author_name, author_handle,
			 content_text, content_lang, post_url, published_at,
			 like_count, reply_count, repost_count, quote_count, view_count, normalized_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	).Scan(&id).Error
	return id, err
}

func (r *ContentRepo) GetPostByPlatformID(ctx context.Context, platform, platformPostID string) (*content.NormalizedPost, error) {
	var p content.NormalizedPost
	err := r.db.WithContext(ctx).Raw(
		`SELECT platform, platform_post_id, author_platform_id, author_name, author_handle,
		        content_text, content_lang, post_url, published_at,
		        like_count, reply_count, repost_count, quote_count, view_count, normalized_hash
		 FROM platform_posts WHERE platform = ? AND platform_post_id = ?`,
		platform, platformPostID,
	).Scan(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && p.Platform == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ContentRepo) UpsertHit(ctx context.Context, hit content.MonitorHit) error {
	kwJSON, _ := json.Marshal(hit.MatchedKeywords)
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO monitor_post_hits (monitor_id, post_id, matched_keywords, relevance_score)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (monitor_id, post_id) DO UPDATE SET
			 matched_keywords = EXCLUDED.matched_keywords,
			 relevance_score = EXCLUDED.relevance_score,
			 last_seen_at = now()`,
		hit.MonitorID, hit.PostID, kwJSON, hit.RelevanceScore,
	).Error
}

func (r *ContentRepo) GetHitsByMonitor(ctx context.Context, monitorID int64) ([]content.MonitorHit, error) {
	rows, err := r.db.WithContext(ctx).Raw(
		`SELECT monitor_id, post_id, matched_keywords, relevance_score
		 FROM monitor_post_hits WHERE monitor_id = ? ORDER BY first_seen_at DESC`, monitorID,
	).Rows()
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
