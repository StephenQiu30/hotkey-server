package database

import (
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
	"gorm.io/gorm"
)

// HitScoreRepo implements scoring.HitRepository via GORM.
type HitScoreRepo struct {
	db *gorm.DB
}

func NewHitScoreRepo(db *gorm.DB) *HitScoreRepo {
	return &HitScoreRepo{db: db}
}

func (r *HitScoreRepo) UpdateScores(hitID int64, score scoring.SavedScore) error {
	return r.db.Exec(
		`UPDATE monitor_post_hits SET
			heat_score = ?, relevance_score = ?, freshness_score = ?,
			author_influence_score = ?, final_score = ?
		 WHERE id = ?`,
		score.HeatScore, score.RelevanceScore, score.FreshnessScore,
		score.AuthorInfluenceScore, score.FinalScore, hitID,
	).Error
}
