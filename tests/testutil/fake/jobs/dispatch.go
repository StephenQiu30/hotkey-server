package fakejobs

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
)

// DeliveryRepo is a fake implementing jobs.DeliveryRepository.
type DeliveryRepo struct {
	LastNotificationID    int64
	LastRecipientEmail    string
	LastStatus            string
	LastProviderMessageID string
	LastSentAt            *time.Time
	Deliveries            []jobs.EmailDelivery
}

func (r *DeliveryRepo) CreateDelivery(_ context.Context, d jobs.EmailDelivery) error {
	r.Deliveries = append(r.Deliveries, d)
	r.LastNotificationID = d.NotificationID
	r.LastRecipientEmail = d.RecipientEmail
	r.LastStatus = d.Status
	return nil
}

func (r *DeliveryRepo) UpdateDeliveryStatus(_ context.Context, notificationID int64, status string, providerMsgID string, _ string) error {
	r.LastNotificationID = notificationID
	r.LastStatus = status
	r.LastProviderMessageID = providerMsgID
	if status == "sent" {
		now := time.Now()
		r.LastSentAt = &now
	}
	return nil
}

func (r *DeliveryRepo) GetPendingDeliveries(_ context.Context, _ int) ([]jobs.EmailDelivery, error) {
	return r.Deliveries, nil
}

// Mailer is a fake implementing notify.Mailer.
type Mailer struct {
	MessageID string
	LastTo    string
	Err       error
}

func (m *Mailer) Send(_ context.Context, to, _, _ string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	m.LastTo = to
	return m.MessageID, nil
}

// EmailResolver is a fake implementing jobs.UserEmailLookup.
type EmailResolver struct {
	Email string
	Err   error
}

func (r *EmailResolver) ResolveEmail(_ context.Context, _ int64) (string, error) {
	if r.Err != nil {
		return "", r.Err
	}
	return r.Email, nil
}
