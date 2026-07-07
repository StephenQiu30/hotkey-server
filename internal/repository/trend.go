package repository

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type TrendRepository interface {
	SaveTopicSnapshot(ctx context.Context, snap model.TopicSnapshot) error
	SaveMonitorSnapshot(ctx context.Context, snap model.MonitorSnapshot) error
	GetPreviousTopicHeat(ctx context.Context, topicID int64) (float64, error)
	GetTopicTrends(ctx context.Context, topicID int64, since time.Time) ([]model.TopicSnapshot, error)
	GetMonitorTrends(ctx context.Context, monitorID int64, since time.Time) ([]model.MonitorSnapshot, error)
}
