package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
	"gorm.io/gorm"
)

type RunRepo struct {
	db *gorm.DB
}

func NewRunRepo(db *gorm.DB) *RunRepo {
	return &RunRepo{db: db}
}

func (r *RunRepo) Create(ctx context.Context, run model.KnowledgeRun) (model.KnowledgeRun, error) {
	m := KnowledgeRun{
		RunKey:  run.RunKey,
		RunType: run.RunType,
		TargetDate: run.TargetDate,
		Status:  run.Status,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return model.KnowledgeRun{}, err
	}
	result := ToKnowledgeRun(m)
	return result, nil
}

func (r *RunRepo) GetByKey(ctx context.Context, runKey string) (*model.KnowledgeRun, error) {
	var m KnowledgeRun
	if err := r.db.WithContext(ctx).Where("run_key = ?", runKey).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	result := ToKnowledgeRun(m)
	return &result, nil
}
