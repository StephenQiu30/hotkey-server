package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/notify"
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

// UserEmailLookup resolves a user's email address from a notification ID.
type UserEmailLookup interface {
	ResolveEmail(ctx context.Context, notificationID int64) (string, error)
}

// DispatchJob orchestrates email delivery for pending notifications.
type DispatchJob struct {
	repo     DeliveryRepository
	mailer   notify.Mailer
	resolver UserEmailLookup
}

// NewDispatchJob creates a new DispatchJob.
func NewDispatchJob(repo DeliveryRepository, mailer notify.Mailer, resolver UserEmailLookup) *DispatchJob {
	return &DispatchJob{repo: repo, mailer: mailer, resolver: resolver}
}

// Run processes pending email deliveries for the given notification.
func (j *DispatchJob) Run(ctx context.Context, notificationID int64) error {
	// Resolve real recipient email from notification
	recipientEmail, err := j.resolver.ResolveEmail(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("resolve recipient email: %w", err)
	}

	// Create delivery record
	delivery := EmailDelivery{
		NotificationID: notificationID,
		RecipientEmail: recipientEmail,
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

// RunPending sends delivery records that were already queued.
func (j *DispatchJob) RunPending(ctx context.Context, limit int) error {
	deliveries, err := j.repo.GetPendingDeliveries(ctx, limit)
	if err != nil {
		return fmt.Errorf("list pending deliveries: %w", err)
	}
	for _, delivery := range deliveries {
		providerMsgID, err := j.mailer.Send(ctx, delivery.RecipientEmail, "Notification", "You have a new alert")
		if err != nil {
			_ = j.repo.UpdateDeliveryStatus(ctx, delivery.NotificationID, "failed", "", err.Error())
			return fmt.Errorf("send email: %w", err)
		}
		if err := j.repo.UpdateDeliveryStatus(ctx, delivery.NotificationID, "sent", providerMsgID, ""); err != nil {
			return fmt.Errorf("update delivery status: %w", err)
		}
	}
	return nil
}
