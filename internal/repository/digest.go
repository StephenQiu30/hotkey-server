package repository

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type DigestRepository interface {
	Upsert(ctx context.Context, e model.TopicDailyExport) (model.TopicDailyExport, error)
	GetByTopicDate(ctx context.Context, topicID int64, exportDate time.Time) (*model.TopicDailyExport, error)
}
