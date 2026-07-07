package repository

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type RunRepository interface {
	Create(ctx context.Context, run model.KnowledgeRun) (model.KnowledgeRun, error)
	GetByKey(ctx context.Context, runKey string) (*model.KnowledgeRun, error)
}
