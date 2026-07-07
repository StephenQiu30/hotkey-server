package repository

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model"
)

type NotifyRepository interface {
	ListUnread(ctx context.Context, userID int64) ([]model.Notification, error)
	MarkRead(ctx context.Context, userID, notificationID int64) error
	Create(ctx context.Context, n model.Notification) (model.Notification, error)
}
