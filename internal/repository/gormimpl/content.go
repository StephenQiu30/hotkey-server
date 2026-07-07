package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PostRepo struct {
	db *gorm.DB
}

func NewPostRepo(db *gorm.DB) *PostRepo {
	return &PostRepo{db: db}
}

func (r *PostRepo) UpsertPost(ctx context.Context, post model.PlatformPost) (int64, error) {
	m := PlatformPost{
		Platform:         post.Platform,
		PlatformPostID:   post.PlatformPostID,
		AuthorPlatformID: post.AuthorPlatformID,
		AuthorName:       post.AuthorName,
		AuthorHandle:     post.AuthorHandle,
		ContentText:      post.ContentText,
		ContentLang:      post.ContentLang,
		PostURL:          post.PostURL,
		PublishedAt:      &post.PublishedAt,
		LikeCount:        post.LikeCount,
		ReplyCount:       post.ReplyCount,
		RepostCount:      post.RepostCount,
		QuoteCount:       post.QuoteCount,
		ViewCount:        post.ViewCount,
		RawPayload:       pkg.JSONB[string]{},
		NormalizedHash:   post.NormalizedHash,
	}

	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "platform"}, {Name: "platform_post_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content_text", "like_count", "reply_count", "repost_count", "quote_count", "view_count", "normalized_hash", "updated_at"}),
		}).
		Create(&m).Error; err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (r *PostRepo) GetPostByPlatformID(ctx context.Context, platform, platformPostID string) (*model.PlatformPost, error) {
	var m PlatformPost
	if err := r.db.WithContext(ctx).
		Where("platform = ? AND platform_post_id = ?", platform, platformPostID).
		First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	result := ToPlatformPost(m)
	return &result, nil
}

type HitRepo struct {
	db *gorm.DB
}

func NewHitRepo(db *gorm.DB) *HitRepo {
	return &HitRepo{db: db}
}

func (r *HitRepo) UpsertHit(ctx context.Context, hit model.MonitorHit) error {
	m := MonitorPostHit{
		MonitorID:           hit.MonitorID,
		PostID:              hit.PostID,
		MatchedKeywords:     hit.MatchedKeywords,
		RelevanceScore:      hit.RelevanceScore,
		HeatScore:           hit.HeatScore,
		FreshnessScore:      hit.FreshnessScore,
		AuthorInfluenceScore: hit.AuthorInfluenceScore,
		FinalScore:          hit.FinalScore,
		FirstSeenAt:         hit.FirstSeenAt,
		LastSeenAt:          hit.LastSeenAt,
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "monitor_id"}, {Name: "post_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"relevance_score", "heat_score", "freshness_score", "author_influence_score", "final_score", "last_seen_at"}),
		}).
		Create(&m).Error
}

func (r *HitRepo) GetHitsByMonitor(ctx context.Context, monitorID int64) ([]model.MonitorHit, error) {
	var models []MonitorPostHit
	if err := r.db.WithContext(ctx).Where("monitor_id = ?", monitorID).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]model.MonitorHit, len(models))
	for i := range models {
		result[i] = model.MonitorHit{
			ID:                  models[i].ID,
			MonitorID:           models[i].MonitorID,
			PostID:              models[i].PostID,
			MatchedKeywords:     models[i].MatchedKeywords,
			RelevanceScore:      models[i].RelevanceScore,
			HeatScore:           models[i].HeatScore,
			FreshnessScore:      models[i].FreshnessScore,
			AuthorInfluenceScore: models[i].AuthorInfluenceScore,
			FinalScore:          models[i].FinalScore,
			FirstSeenAt:         models[i].FirstSeenAt,
			LastSeenAt:          models[i].LastSeenAt,
		}
	}
	return result, nil
}

// GetHitByPostID retrieves a hit by (monitor_id, post_id).
func (r *HitRepo) GetHitByPostID(ctx context.Context, monitorID, postID int64) (*model.MonitorHit, error) {
	var m MonitorPostHit
	if err := r.db.WithContext(ctx).
		Where("monitor_id = ? AND post_id = ?", monitorID, postID).
		First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &model.MonitorHit{
		ID:        m.ID,
		MonitorID: m.MonitorID,
		PostID:    m.PostID,
	}, nil
}

// UpdateScores updates hit scores by post_id (Bug #2 fix — was using wrong id).
func (r *HitRepo) UpdateScores(ctx context.Context, postID int64, heat, relevance, freshness, finalScore float64) error {
	return r.db.WithContext(ctx).Model(&MonitorPostHit{}).
		Where("post_id = ?", postID).
		Updates(map[string]any{
			"heat_score":      heat,
			"relevance_score": relevance,
			"freshness_score": freshness,
			"final_score":     finalScore,
		}).Error
}
