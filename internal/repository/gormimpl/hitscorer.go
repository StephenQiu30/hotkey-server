package gormimpl

import (
	"context"

	"gorm.io/gorm"
)

// HitScoreRepo implements scoring.HitRepository via GORM.
// Fixes Bug #2: Updates by post_id instead of hit_id.
type HitScoreRepo struct {
	db *gorm.DB
}

func NewHitScoreRepo(db *gorm.DB) *HitScoreRepo {
	return &HitScoreRepo{db: db}
}

// UpdateScores updates all score columns for a given monitor_post_hit row by post_id.
func (r *HitScoreRepo) UpdateScores(ctx context.Context, postID int64, heatScore, relevanceScore, freshnessScore, authorInfluenceScore, finalScore float64) error {
	return r.db.WithContext(ctx).Model(&MonitorPostHit{}).
		Where("post_id = ?", postID).
		Updates(map[string]any{
			"heat_score":              heatScore,
			"relevance_score":         relevanceScore,
			"freshness_score":         freshnessScore,
			"author_influence_score":  authorInfluenceScore,
			"final_score":             finalScore,
		}).Error
}
