package notify

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

// Service provides notification operations.
type Service struct {
	repo Repository
}

// NewService creates a new notify Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListUnread returns unread notifications for a user, newest first.
func (s *Service) ListUnread(ctx context.Context, userID int64) ([]dto.Notification, error) {
	return s.repo.ListUnread(ctx, userID)
}

// MarkRead marks a notification as read for the given user.
func (s *Service) MarkRead(ctx context.Context, userID, notificationID int64) error {
	return s.repo.MarkRead(ctx, userID, notificationID)
}
