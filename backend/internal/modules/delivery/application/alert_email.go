package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/domain"
)

type AlertEmailStore interface {
	GetAlertDelivery(context.Context, int64) (domain.AlertDelivery, error)
	ClaimAlertDelivery(context.Context, int64) (domain.AlertDelivery, error)
	UpdateAlertDelivery(context.Context, domain.AlertDelivery) error
	AppendAlertAttempt(context.Context, int64, int, string, string) error
}

type AlertEmailService struct {
	store AlertEmailStore
	mail  MailSender
	now   func() time.Time
}

func NewAlertEmailService(store AlertEmailStore, mail MailSender, now func() time.Time) (*AlertEmailService, error) {
	if store == nil || mail == nil {
		return nil, fmt.Errorf("alert email service dependencies are required")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &AlertEmailService{store: store, mail: mail, now: now}, nil
}

func (service *AlertEmailService) Deliver(ctx context.Context, deliveryID int64) error {
	if service == nil || deliveryID <= 0 {
		return fmt.Errorf("invalid alert delivery request")
	}
	delivery, err := service.store.ClaimAlertDelivery(ctx, deliveryID)
	if err != nil {
		return err
	}
	attemptNo := delivery.AttemptCount + 1
	if attemptNo < 1 || attemptNo > 5 {
		return fmt.Errorf("alert email delivery exhausted")
	}
	if err := service.store.AppendAlertAttempt(ctx, delivery.ID, attemptNo, "started", ""); err != nil {
		return err
	}
	err = service.mail.Send(ctx, MailMessage{To: delivery.Recipient, Subject: delivery.Subject, Text: delivery.TextBody, HTML: delivery.HTMLBody})
	if err == nil {
		if appendErr := service.store.AppendAlertAttempt(ctx, delivery.ID, attemptNo, "succeeded", ""); appendErr != nil {
			return appendErr
		}
		now := service.now().UTC()
		delivery.Status, delivery.SucceededAt, delivery.NextAttemptAt, delivery.LastError = domain.DeliverySucceeded, &now, nil, ""
		return service.store.UpdateAlertDelivery(ctx, delivery)
	}
	temporary := true
	var failure interface{ TemporaryFailure() bool }
	if errors.As(err, &failure) {
		temporary = failure.TemporaryFailure()
	}
	message := deliveryErrorMessage(temporary)
	_ = service.store.AppendAlertAttempt(ctx, delivery.ID, attemptNo, "failed", message)
	delivery.Status, delivery.LastError = domain.DeliveryFailed, message
	delivery.NextAttemptAt = nil
	if temporary && attemptNo < 5 {
		delivery.Status = domain.DeliveryRetrying
		next := service.now().UTC().Add(backoff(attemptNo))
		delivery.NextAttemptAt = &next
	}
	if updateErr := service.store.UpdateAlertDelivery(ctx, delivery); updateErr != nil {
		return updateErr
	}
	return err
}
