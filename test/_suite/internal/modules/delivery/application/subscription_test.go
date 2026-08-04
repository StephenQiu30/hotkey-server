package application_test

import (
	"context"
	"errors"
	"testing"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

type unusedSubscriptionStore struct{}

func (unusedSubscriptionStore) CreateSubscription(context.Context, domain.Subscription) (domain.Subscription, error) {
	panic("unexpected call")
}
func (unusedSubscriptionStore) GetSubscription(context.Context, int64, int64) (domain.Subscription, error) {
	panic("unexpected call")
}
func (unusedSubscriptionStore) ListSubscriptions(context.Context, int64, domain.SubscriptionListQuery) (domain.SubscriptionPage, error) {
	panic("unexpected call")
}
func (unusedSubscriptionStore) UpdateSubscription(context.Context, domain.Subscription, int64) (domain.Subscription, error) {
	panic("unexpected call")
}
func (unusedSubscriptionStore) RotateRSSToken(context.Context, int64, int64, int64, string) (domain.Subscription, error) {
	panic("unexpected call")
}
func (unusedSubscriptionStore) DeleteSubscription(context.Context, int64, int64, int64) (domain.Subscription, error) {
	panic("unexpected call")
}

type unusedAuditWriter struct{}

func (unusedAuditWriter) Write(context.Context, operationsdomain.AuditEntry) error {
	panic("unexpected call")
}

func TestSubscriptionServiceMapsInvalidCreateInputToBadRequest(t *testing.T) {
	service, err := deliveryapplication.NewSubscriptionService(deliveryapplication.SubscriptionDependencies{
		Runtime: &database.Runtime{},
		Store:   unusedSubscriptionStore{},
		Audit:   unusedAuditWriter{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Create(context.Background(), deliveryapplication.CreateSubscriptionInput{
		Subject: identitydomain.Subject{UserID: 1, SessionID: 2, Role: identitydomain.RoleViewer},
	})
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) {
		t.Fatalf("Create() error = %T %v, want AppError", err, err)
	}
	if appError.Code != sharederrors.CodeInvalidRequest || appError.HTTPStatus != 400 {
		t.Fatalf("Create() error = code %d/status %d, want %d/400", appError.Code, appError.HTTPStatus, sharederrors.CodeInvalidRequest)
	}
}
