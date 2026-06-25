package database

import (
	"context"
	"encoding/json"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"gorm.io/gorm"
)

// PollPostRepo implements jobs.PostRepository via GORM.
type PollPostRepo struct {
	db *gorm.DB
}

func NewPollPostRepo(db *gorm.DB) *PollPostRepo {
	return &PollPostRepo{db: db}
}

func (r *PollPostRepo) UpsertPost(ctx context.Context, post jobs.PostResult) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO platform_posts
			(platform, platform_post_id, author_platform_id, author_name, author_handle,
			 content_text, content_lang, published_at,
			 like_count, reply_count, repost_count, quote_count, view_count)
		 VALUES ('x', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		post.ID, post.AuthorID, post.AuthorName, post.AuthorHandle,
		post.Text, post.Language, post.PublishedAt,
		post.LikeCount, post.ReplyCount, post.RepostCount, post.QuoteCount, post.ViewCount,
	).Scan(&id).Error
	return id, err
}

// PollHitRepo implements jobs.HitRepository via GORM.
type PollHitRepo struct {
	db *gorm.DB
}

func NewPollHitRepo(db *gorm.DB) *PollHitRepo {
	return &PollHitRepo{db: db}
}

func (r *PollHitRepo) UpsertHit(ctx context.Context, hit jobs.HitResult) error {
	kwJSON, _ := json.Marshal(hit.MatchedKeywords)
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO monitor_post_hits (monitor_id, post_id, matched_keywords)
		 VALUES (?, ?, ?)
		 ON CONFLICT (monitor_id, post_id) DO UPDATE SET
			 matched_keywords = EXCLUDED.matched_keywords,
			 last_seen_at = now()`,
		hit.MonitorID, hit.PostID, kwJSON,
	).Error
}
