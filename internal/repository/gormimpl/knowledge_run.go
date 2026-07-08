package gormimpl

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type KnowledgeRunRepo struct {
	db *gorm.DB
}

func NewKnowledgeRunRepo(db *gorm.DB) *KnowledgeRunRepo {
	return &KnowledgeRunRepo{db: db}
}

func (r *KnowledgeRunRepo) TryStart(ctx context.Context, runKey string, runType string, targetDate time.Time, startedAt time.Time) (bool, error) {
	model := KnowledgeRun{
		RunKey:     runKey,
		RunType:    runType,
		TargetDate: &targetDate,
		Status:     "running",
		StartedAt:  &startedAt,
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *KnowledgeRunRepo) MarkFinished(ctx context.Context, runKey string, finishedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&KnowledgeRun{}).Where("run_key = ?", runKey).Updates(map[string]any{
		"status":      "finished",
		"finished_at": finishedAt,
	}).Error
}

func (r *KnowledgeRunRepo) MarkFailed(ctx context.Context, runKey string, message string, failedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&KnowledgeRun{}).Where("run_key = ?", runKey).Updates(map[string]any{
		"status":        "failed",
		"error_message": message,
		"finished_at":   failedAt,
	}).Error
}
