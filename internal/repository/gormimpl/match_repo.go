package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"gorm.io/gorm"
)

// PostMatch represents a post with its cosine similarity score.
type PostMatch struct {
	PlatformPost
	Similarity float64 `gorm:"-:all" json:"similarity"`
}

// MatchRepo handles pgvector cosine similarity queries.
type MatchRepo struct {
	db *gorm.DB
}

func NewMatchRepo(db *gorm.DB) *MatchRepo {
	return &MatchRepo{db: db}
}

// FindMatchingPosts returns posts matching a monitor's query_embedding above threshold.
func (r *MatchRepo) FindMatchingPosts(ctx context.Context, monitorID int64, threshold float64, limit int) ([]PostMatch, error) {
	var results []PostMatch
	err := r.db.WithContext(ctx).
		Table("platform_posts").
		Select("platform_posts.*, 1 - (platform_posts.embedding <=> ?) AS similarity",
			gorm.Expr("keyword_monitors.query_embedding")).
		Joins("JOIN keyword_monitors ON keyword_monitors.id = ?", monitorID).
		Where("1 - (platform_posts.embedding <=> keyword_monitors.query_embedding) >= ?", threshold).
		Where("platform_posts.embedding IS NOT NULL").
		Where("keyword_monitors.query_embedding IS NOT NULL").
		Order(gorm.Expr("similarity DESC")).
		Limit(limit).
		Find(&results).Error
	return results, err
}

// UpdatePostEmbedding updates the embedding for an existing post.
func (r *MatchRepo) UpdatePostEmbedding(ctx context.Context, postID int64, emb pkg.Vector384) error {
	return r.db.WithContext(ctx).Model(&PlatformPost{}).
		Where("id = ?", postID).
		Update("embedding", emb).Error
}
