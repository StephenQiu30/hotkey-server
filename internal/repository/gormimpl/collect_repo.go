package gormimpl

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"gorm.io/gorm"
)

// CollectRepo handles collection-related DB writes (posts, hits, monitors).
type CollectRepo struct {
	db *gorm.DB
}

func NewCollectRepo(db *gorm.DB) *CollectRepo {
	return &CollectRepo{db: db}
}

// UpsertPost creates or updates a post by platform + platform_post_id.
func (r *CollectRepo) UpsertPost(ctx context.Context, p *entity.PlatformPost) error {
	if p.Platform == "" || p.PlatformPostID == "" {
		return nil
	}
	// Try to find existing
	var existing entity.PlatformPost
	err := r.db.WithContext(ctx).
		Where("platform = ? AND platform_post_id = ?", p.Platform, p.PlatformPostID).
		First(&existing).Error
	if err == nil {
		// Update existing
		updates := map[string]interface{}{
			"content_text": p.ContentText,
			"author_name":  p.AuthorName,
			"author_handle": p.AuthorHandle,
			"updated_at":   time.Now(),
		}
		if p.Embedding != nil {
			updates["embedding"] = *p.Embedding
		}
		if p.PublishedAt != nil {
			updates["published_at"] = p.PublishedAt
		}
		if p.ContentLang != "" {
			updates["content_lang"] = p.ContentLang
		}
		p.ID = existing.ID
		return r.db.WithContext(ctx).Model(&existing).Updates(updates).Error
	}
	// Create new
	return r.db.WithContext(ctx).Create(p).Error
}

// CreateHit records a monitor_post_hit entry.
func (r *CollectRepo) CreateHit(ctx context.Context, hit *entity.MonitorPostHit) error {
	return r.db.WithContext(ctx).Create(hit).Error
}

// ListActiveMonitors retrieves all active monitors with their query embeddings.
func (r *CollectRepo) ListActiveMonitors(ctx context.Context) ([]entity.KeywordMonitor, error) {
	var monitors []entity.KeywordMonitor
	if err := r.db.WithContext(ctx).
		Where("status = ?", "active").
		Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// ListHitsSince retrieves monitor_post_hits created after a given time.
func (r *CollectRepo) ListHitsSince(ctx context.Context, since time.Time) ([]entity.MonitorPostHit, error) {
	var hits []entity.MonitorPostHit
	if err := r.db.WithContext(ctx).
		Where("first_seen_at >= ?", since).
		Find(&hits).Error; err != nil {
		return nil, err
	}
	return hits, nil
}
