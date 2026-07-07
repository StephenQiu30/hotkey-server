package model

import "time"

// Notification is a user-facing alert notification.
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
