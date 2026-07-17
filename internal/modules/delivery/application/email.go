package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
)

type MailMessage struct {
	To, Subject, HTML, Text string
}

type MailSender interface {
	Send(context.Context, MailMessage) error
}

type EmailStore interface {
	GetDelivery(context.Context, int64) (domain.Delivery, error)
	ClaimDelivery(context.Context, int64) (domain.Delivery, error)
	UpdateDelivery(context.Context, domain.Delivery) error
	AppendAttempt(context.Context, int64, int, string, int, string) error
}

type EmailService struct {
	store EmailStore
	mail  MailSender
	now   func() time.Time
}

func NewEmailService(store EmailStore, mail MailSender, now func() time.Time) (*EmailService, error) {
	if store == nil || mail == nil {
		return nil, fmt.Errorf("email service dependencies are required")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &EmailService{store: store, mail: mail, now: now}, nil
}

func (service *EmailService) Deliver(ctx context.Context, deliveryID int64, message MailMessage, attemptNo int) error {
	if service == nil || deliveryID <= 0 || attemptNo <= 0 {
		return fmt.Errorf("invalid delivery request")
	}
	delivery, err := service.store.ClaimDelivery(ctx, deliveryID)
	if err != nil {
		return err
	}
	if err := service.store.AppendAttempt(ctx, delivery.ID, attemptNo, "started", 0, ""); err != nil {
		return err
	}
	err = service.mail.Send(ctx, message)
	if err == nil {
		if appendErr := service.store.AppendAttempt(ctx, delivery.ID, attemptNo, "succeeded", 250, ""); appendErr != nil {
			return appendErr
		}
		now := service.now().UTC()
		delivery.Status, delivery.SucceededAt, delivery.NextAttemptAt = domain.DeliverySucceeded, &now, nil
		return service.store.UpdateDelivery(ctx, delivery)
	}
	temporary := true
	var failure interface{ TemporaryFailure() bool }
	if errors.As(err, &failure) {
		temporary = failure.TemporaryFailure()
	}
	status := domain.DeliveryFailed
	var next *time.Time
	if temporary {
		status = domain.DeliveryRetrying
		value := service.now().UTC().Add(backoff(attemptNo))
		next = &value
	}
	_ = service.store.AppendAttempt(ctx, delivery.ID, attemptNo, "failed", 0, deliveryErrorMessage(temporary))
	delivery.Status, delivery.NextAttemptAt = status, next
	if updateErr := service.store.UpdateDelivery(ctx, delivery); updateErr != nil {
		return updateErr
	}
	return err
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<uint(attempt-1)) * time.Minute
}

func deliveryErrorMessage(temporary bool) string {
	if temporary {
		return "temporary smtp failure"
	}
	return "permanent smtp failure"
}
