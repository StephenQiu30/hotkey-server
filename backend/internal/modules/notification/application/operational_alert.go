package application

import (
	"context"
	"time"
)

// UnknownDeliveryAlertSummary is the bounded, non-sensitive notification fact
// exposed to the Operations application. Provider identifiers, dispatch keys,
// recipients, content and error details intentionally stay inside Notification.
type UnknownDeliveryAlertSummary struct {
	AttemptID      int64
	NotificationID int64
	ResourceType   string
	ResourceID     int64
	AffectedCount  int64
	TriggeredAt    time.Time
}

type UnknownDeliveryAlertReader interface {
	UnknownDeliveryAlert(context.Context) (UnknownDeliveryAlertSummary, bool, error)
}
