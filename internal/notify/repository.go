package notify

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

// Sentinel errors for notify operations.
var (
	ErrNotFound = errors.New("notification not found")
	ErrNotOwned = errors.New("notification not owned by user")
)

// Repository defines the persistence interface for notification operations.
type Repository interface {
	// ListUnread returns unread notifications for a user, newest first.
	ListUnread(ctx context.Context, userID int64) ([]dto.Notification, error)

	// MarkRead marks a notification as read. Returns ErrNotOwned if the
	// notification belongs to a different user.
	MarkRead(ctx context.Context, userID, notificationID int64) error

	// Create inserts a new notification.
	Create(ctx context.Context, n dto.Notification) (dto.Notification, error)
}
