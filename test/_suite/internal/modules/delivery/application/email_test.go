package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type emailStoreFake struct {
	delivery domain.Delivery
	attempts []string
}

func (fake *emailStoreFake) GetDelivery(context.Context, int64) (domain.Delivery, error) {
	return fake.delivery, nil
}
func (fake *emailStoreFake) ClaimDelivery(context.Context, int64) (domain.Delivery, error) {
	if fake.delivery.Status == domain.DeliverySucceeded {
		return domain.Delivery{}, sharedrepository.ErrConflict
	}
	fake.delivery.Status = domain.DeliveryClaimed
	return fake.delivery, nil
}
func (fake *emailStoreFake) UpdateDelivery(_ context.Context, delivery domain.Delivery) error {
	fake.delivery = delivery
	return nil
}
func (fake *emailStoreFake) AppendAttempt(_ context.Context, _ int64, attempt int, status string, _ int, message string) error {
	fake.attempts = append(fake.attempts, status+":"+message)
	if attempt <= 0 {
		return errors.New("invalid attempt")
	}
	return nil
}

type mailFake struct{ err error }

func (fake mailFake) Send(context.Context, MailMessage) error { return fake.err }

type temporaryMailError struct{}

func (temporaryMailError) Error() string          { return "temporary" }
func (temporaryMailError) TemporaryFailure() bool { return true }

func TestEmailServiceMarksSuccessAndAppendsAttempts(t *testing.T) {
	store := &emailStoreFake{delivery: domain.Delivery{ID: 7, ReportID: 8, SubscriptionID: 9, IdempotencyKey: "8:9", Status: domain.DeliveryQueued}}
	service, err := NewEmailService(store, mailFake{}, func() time.Time { return time.Unix(10, 0).UTC() })
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Deliver(context.Background(), 7, MailMessage{To: "a@example.test", Subject: "report", Text: "body"}, 1); err != nil {
		t.Fatal(err)
	}
	if store.delivery.Status != domain.DeliverySucceeded || len(store.attempts) != 2 {
		t.Fatalf("delivery = %#v, attempts = %#v", store.delivery, store.attempts)
	}
}

func TestEmailServiceRetriesTemporaryFailureWithoutLeakingError(t *testing.T) {
	store := &emailStoreFake{delivery: domain.Delivery{ID: 7, ReportID: 8, SubscriptionID: 9, IdempotencyKey: "8:9", Status: domain.DeliveryQueued}}
	service, err := NewEmailService(store, mailFake{err: temporaryMailError{}}, func() time.Time { return time.Unix(10, 0).UTC() })
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Deliver(context.Background(), 7, MailMessage{To: "a@example.test", Subject: "report", Text: "body"}, 2); err == nil {
		t.Fatal("temporary SMTP failure was swallowed")
	}
	if store.delivery.Status != domain.DeliveryRetrying || store.delivery.NextAttemptAt == nil || store.attempts[1] != "failed:temporary smtp failure" {
		t.Fatalf("retry delivery = %#v, attempts = %#v", store.delivery, store.attempts)
	}
}
