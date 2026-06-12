package jobs

import (
	"context"
	"fmt"
	"time"
)

// EmailDelivery represents an email delivery record.
type EmailDelivery struct {
	ID                int64
	NotificationID    int64
	RecipientEmail    string
	Provider          string
	ProviderMessageID string
	Status            string
	ErrorMessage      string
	SentAt            *time.Time
}

// DeliveryRepository abstracts persistence for email delivery records.
type DeliveryRepository interface {
	// CreateDelivery inserts a new delivery record.
	CreateDelivery(ctx context.Context, d EmailDelivery) error

	// UpdateDeliveryStatus updates the status of a delivery record.
	UpdateDeliveryStatus(ctx context.Context, notificationID int64, status string, providerMsgID string, errMsg string) error

	// GetPendingDeliveries returns pending delivery records up to limit.
	GetPendingDeliveries(ctx context.Context, limit int) ([]EmailDelivery, error)
}

// Mailer abstracts email sending.
type Mailer interface {
	// Send sends an email and returns the provider message ID.
	Send(ctx context.Context, to, subject, body string) (string, error)
}

// DispatchJob orchestrates email delivery for pending notifications.
type DispatchJob struct {
	repo   DeliveryRepository
	mailer Mailer
}

// NewDispatchJob creates a new DispatchJob.
func NewDispatchJob(repo DeliveryRepository, mailer Mailer) *DispatchJob {
	return &DispatchJob{repo: repo, mailer: mailer}
}

// Run processes pending email deliveries for the given notification.
func (j *DispatchJob) Run(ctx context.Context, notificationID int64) error {
	// Create delivery record
	delivery := EmailDelivery{
		NotificationID: notificationID,
		RecipientEmail: "user@example.com", // placeholder; real impl would look up
		Provider:       "smtp",
		Status:         "pending",
	}
	if err := j.repo.CreateDelivery(ctx, delivery); err != nil {
		return fmt.Errorf("create delivery: %w", err)
	}

	// Attempt to send
	providerMsgID, err := j.mailer.Send(ctx, delivery.RecipientEmail, "Notification", "You have a new alert")
	if err != nil {
		_ = j.repo.UpdateDeliveryStatus(ctx, notificationID, "failed", "", err.Error())
		return fmt.Errorf("send email: %w", err)
	}

	// Mark as sent
	if err := j.repo.UpdateDeliveryStatus(ctx, notificationID, "sent", providerMsgID, ""); err != nil {
		return fmt.Errorf("update delivery status: %w", err)
	}

	return nil
}
