package report

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

type Repository interface {
	ListUserMonitors(ctx context.Context, userID int64) ([]dto.MonitorSource, error)
	ListTopics(ctx context.Context, monitorIDs []int64, start, end time.Time, limit int) ([]dto.TopicSource, error)
	ListPosts(ctx context.Context, monitorIDs []int64, start, end time.Time, limit int) ([]dto.PostSource, error)
	Create(ctx context.Context, in dto.CreateReportRecord) (dto.Report, error)
	List(ctx context.Context, filter dto.ListFilter) ([]dto.Report, int64, error)
	GetByID(ctx context.Context, id, userID int64) (dto.Report, error)
	MarkSent(ctx context.Context, id, userID int64, sentAt time.Time) (dto.Report, error)
}
