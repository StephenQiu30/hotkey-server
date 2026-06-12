package content

import "context"

// PostRepository persists normalized posts to the database.
type PostRepository interface {
	UpsertPost(ctx context.Context, post NormalizedPost) (int64, error)
	GetPostByPlatformID(ctx context.Context, platform, platformPostID string) (*NormalizedPost, error)
}

// HitRepository persists monitor-post hit relationships.
type HitRepository interface {
	UpsertHit(ctx context.Context, hit MonitorHit) error
	GetHitsByMonitor(ctx context.Context, monitorID int64) ([]MonitorHit, error)
}
