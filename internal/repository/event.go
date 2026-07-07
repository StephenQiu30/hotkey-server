package repository

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type EventRepository interface {
	Create(ctx context.Context, e model.Event) (model.Event, error)
	GetByID(ctx context.Context, id int64) (*model.Event, error)
	GetByKey(ctx context.Context, monitorID int64, eventKey string) (*model.Event, error)
}
