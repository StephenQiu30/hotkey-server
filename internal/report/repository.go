package report

import (
	"context"
	"time"
)

type Repository interface {
	ListUserMonitors(ctx context.Context, userID int64) ([]MonitorSource, error)
	ListTopics(ctx context.Context, monitorIDs []int64, start, end time.Time, limit int) ([]TopicSource, error)
	ListPosts(ctx context.Context, monitorIDs []int64, start, end time.Time, limit int) ([]PostSource, error)
	Create(ctx context.Context, in CreateReportRecord) (Report, error)
	List(ctx context.Context, filter ListFilter) ([]Report, int64, error)
	GetByID(ctx context.Context, id, userID int64) (Report, error)
	MarkSent(ctx context.Context, id, userID int64, sentAt time.Time) (Report, error)
}
