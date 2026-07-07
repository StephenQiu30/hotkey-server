package repository

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type MonitorRepository interface {
	Create(ctx context.Context, userID int64, input model.CreateMonitorInput) (model.KeywordMonitor, error)
	GetByID(ctx context.Context, id int64) (*model.KeywordMonitor, error)
	ListByUser(ctx context.Context, userID int64) ([]model.KeywordMonitor, error)
	Update(ctx context.Context, id int64, userID int64, input model.UpdateMonitorInput) (model.KeywordMonitor, error)
	ListActiveIDs(ctx context.Context) ([]int64, error)
}
