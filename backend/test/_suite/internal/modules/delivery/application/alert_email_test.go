package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/domain"
)

type alertEmailStoreFake struct {
	delivery domain.AlertDelivery
	attempts []string
}

func (fake *alertEmailStoreFake) GetAlertDelivery(context.Context, int64) (domain.AlertDelivery, error) {
	return fake.delivery, nil
}
func (fake *alertEmailStoreFake) ClaimAlertDelivery(context.Context, int64) (domain.AlertDelivery, error) {
	fake.delivery.Status = domain.DeliveryClaimed
	return fake.delivery, nil
}
func (fake *alertEmailStoreFake) UpdateAlertDelivery(_ context.Context, delivery domain.AlertDelivery) error {
	fake.delivery = delivery
	return nil
}
func (fake *alertEmailStoreFake) AppendAlertAttempt(_ context.Context, _ int64, _ int, status, message string) error {
	fake.attempts = append(fake.attempts, status+":"+message)
	return nil
}

func validAlertDelivery() domain.AlertDelivery {
	return domain.AlertDelivery{ID: 7, OccurrenceID: 8, IdempotencyKey: strings.Repeat("a", 64), Recipient: "owner@example.test", Subject: "HotKey alert", TextBody: "body", HTMLBody: "<p>body</p>", Severity: "critical", Status: domain.DeliveryQueued}
}

func TestAlertEmailServiceMarksSuccessWithoutRebuildingMessage(t *testing.T) {
	store := &alertEmailStoreFake{delivery: validAlertDelivery()}
	service, err := NewAlertEmailService(store, mailFake{}, func() time.Time { return time.Unix(10, 0).UTC() })
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Deliver(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if store.delivery.Status != domain.DeliverySucceeded || store.delivery.SucceededAt == nil || len(store.attempts) != 2 {
		t.Fatalf("delivery = %#v attempts = %#v", store.delivery, store.attempts)
	}
}

func TestAlertEmailServiceStopsAfterFifthTemporaryFailure(t *testing.T) {
	delivery := validAlertDelivery()
	delivery.AttemptCount = 4
	store := &alertEmailStoreFake{delivery: delivery}
	service, err := NewAlertEmailService(store, mailFake{err: temporaryMailError{}}, func() time.Time { return time.Unix(10, 0).UTC() })
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Deliver(context.Background(), 7); err == nil {
		t.Fatal("SMTP failure was swallowed")
	}
	if store.delivery.Status != domain.DeliveryFailed || store.delivery.NextAttemptAt != nil || store.delivery.LastError != "temporary smtp failure" {
		t.Fatalf("final delivery = %#v", store.delivery)
	}
}
