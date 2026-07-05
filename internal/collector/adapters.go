package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/connector"
	"gorm.io/gorm"
)

// TrendingPostRepo implements PostRepository via GORM.
type TrendingPostRepo struct {
	db *gorm.DB
}

func NewTrendingPostRepo(db *gorm.DB) *TrendingPostRepo {
	return &TrendingPostRepo{db: db}
}

// UpsertTrendingPost inserts or updates a trending item as a platform_post.
// Uses the platform + title hash as the conflict key.
func (r *TrendingPostRepo) UpsertTrendingPost(ctx context.Context, item connector.TrendingItem, platformID string) error {
	now := time.Now()

	return r.db.WithContext(ctx).Exec(`
		INSERT INTO platform_posts
			(platform, platform_post_id, content_text, post_url, published_at,
			 like_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT (platform, platform_post_id) DO UPDATE SET
			content_text = EXCLUDED.content_text,
			post_url = EXCLUDED.post_url,
			updated_at = EXCLUDED.updated_at
	`, platformID, fmt.Sprintf("%s-%d", platformID, item.Rank),
		item.Title, item.URL, item.PublishedAt,
		now, now,
	).Error
}
