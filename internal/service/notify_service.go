package service

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

// Notify sentinel errors.
var (
	NotifyErrNotFound = errors.New("notification not found")
	NotifyErrNotOwned       = errors.New("notification not owned by user")
)

// NotifyRepository defines the persistence interface for notification operations.
type NotifyRepository interface {
	ListUnread(ctx context.Context, userID int64) ([]dto.Notification, error)
	MarkRead(ctx context.Context, userID, notificationID int64) error
	Create(ctx context.Context, n dto.Notification) (dto.Notification, error)
}

// Mailer abstracts email sending.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) (string, error)
}

// NotifyService provides notification operations.
type NotifyService struct {
	repo NotifyRepository
}

// NewNotifyService creates a new notify Service.
func NewNotifyService(repo NotifyRepository) *NotifyService {
	return &NotifyService{repo: repo}
}

// ListUnread returns unread notifications for a user, newest first.
func (s *NotifyService) ListUnread(ctx context.Context, userID int64) ([]dto.Notification, error) {
	return s.repo.ListUnread(ctx, userID)
}

// MarkRead marks a notification as read for the given user.
func (s *NotifyService) MarkRead(ctx context.Context, userID, notificationID int64) error {
	return s.repo.MarkRead(ctx, userID, notificationID)
}
