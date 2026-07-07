package repository

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type PostRepository interface {
	UpsertPost(ctx context.Context, post model.PlatformPost) (int64, error)
	GetPostByPlatformID(ctx context.Context, platform, platformPostID string) (*model.PlatformPost, error)
}

type HitRepository interface {
	UpsertHit(ctx context.Context, hit model.MonitorHit) error
	GetHitsByMonitor(ctx context.Context, monitorID int64) ([]model.MonitorHit, error)
	GetHitByPostID(ctx context.Context, monitorID, postID int64) (*model.MonitorHit, error)
	UpdateScores(ctx context.Context, postID int64, heat, relevance, freshness, finalScore float64) error
}
