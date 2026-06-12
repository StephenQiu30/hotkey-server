package notify

import "context"

// Repository defines the persistence interface for notification operations.
type Repository interface {
	// ListUnread returns unread notifications for a user, newest first.
	ListUnread(ctx context.Context, userID int64) ([]Notification, error)

	// MarkRead marks a notification as read. Returns ErrNotOwned if the
	// notification belongs to a different user.
	MarkRead(ctx context.Context, userID, notificationID int64) error

	// Create inserts a new notification.
	Create(ctx context.Context, n Notification) (Notification, error)
}
