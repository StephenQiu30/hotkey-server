package dto

import "time"

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
