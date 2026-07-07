package repository

import (
	"context"
	"time"
)

type DeliveryRepository interface {
	Create(ctx context.Context, notificationID int64, recipientEmail, provider string) (int64, error)
	UpdateStatus(ctx context.Context, id int64, status, providerMessageID, errorMessage string, sentAt *time.Time) error
}
