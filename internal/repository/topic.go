package repository

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type TopicRepository interface {
	UpsertTopic(ctx context.Context, monitorID int64, topicKey, title, summary string, heatScore float64) (int64, error)
	AddPostToTopic(ctx context.Context, topicID, postID int64, membershipScore float64) error
	ListByMonitor(ctx context.Context, monitorID int64) ([]model.TopicSummary, error)
	GetByID(ctx context.Context, id int64) (*model.TopicSummary, error)
}
