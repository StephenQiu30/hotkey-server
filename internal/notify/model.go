// Package notify implements user notification management and delivery.
package notify

import (
	"errors"
	"time"
)

// Sentinel errors for notify operations.
var (
	ErrNotFound = errors.New("notification not found")
	ErrNotOwned = errors.New("notification not owned by user")
)

// Notification represents a user notification linked to an alert.
type Notification struct {
	ID             int64
	UserID         int64
	AlertID        int64
	Channel        string
	DeliveryStatus string
	ReadAt         *time.Time
	SentAt         *time.Time
	CreatedAt      time.Time
}
