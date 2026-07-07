package repository

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type HotEventFilter struct {
	Status   string
	Platform string
	Sort     string
	Limit    int
	Offset   int
}

type HotEventRepository interface {
	Create(ctx context.Context, event *model.HotEvent) error
	GetByID(ctx context.Context, id int64) (*model.HotEvent, error)
	List(ctx context.Context, filter HotEventFilter) ([]*model.HotEvent, int64, error)
	Update(ctx context.Context, event *model.HotEvent) error
	ArchiveOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	AddPlatform(ctx context.Context, eventID int64, platform *model.EventPlatform) error
	GetPlatforms(ctx context.Context, eventID int64) ([]*model.EventPlatform, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}
