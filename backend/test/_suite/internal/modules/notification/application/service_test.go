package application

import (
	"context"
	"errors"
	"testing"
	"time"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type notificationRepositoryStub struct {
	query    ListUserNotificationsQuery
	page     ListUserNotificationsResult
	delivery RecordNotificationDeliveryAttemptCommand
	err      error
}

func (stub *notificationRepositoryStub) ListUserNotifications(_ context.Context, query ListUserNotificationsQuery) (ListUserNotificationsResult, error) {
	stub.query = query
	return stub.page, stub.err
}

func (stub *notificationRepositoryStub) RecordDeliveryAttempt(_ context.Context, command RecordNotificationDeliveryAttemptCommand) (RecordNotificationDeliveryAttemptResult, error) {
	stub.delivery = command
	return RecordNotificationDeliveryAttemptResult{DeliveryAttemptID: 1, AttemptNo: 1}, stub.err
}

func TestServiceNormalizesUserNotificationListAndPreservesIdentity(t *testing.T) {
	repository := &notificationRepositoryStub{page: ListUserNotificationsResult{NextAfterID: 9}}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	page, err := service.ListUserNotifications(context.Background(), ListUserNotificationsQuery{UserID: 7})
	if err != nil || page.NextAfterID != 9 {
		t.Fatalf("ListUserNotifications() = %#v/%v", page, err)
	}
	if repository.query.UserID != 7 || repository.query.Limit != 50 {
		t.Fatalf("repository query = %#v", repository.query)
	}
}

func TestServiceRejectsInvalidUserBeforeRepository(t *testing.T) {
	repository := &notificationRepositoryStub{}
	service, _ := NewService(repository)
	if _, err := service.ListUserNotifications(context.Background(), ListUserNotificationsQuery{}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("ListUserNotifications() error = %v, want invalid input", err)
	}
	if repository.query.UserID != 0 {
		t.Fatalf("repository unexpectedly called with %#v", repository.query)
	}
}

func TestServiceValidatesIndependentDeliveryAttempt(t *testing.T) {
	repository := &notificationRepositoryStub{}
	service, _ := NewService(repository)
	command := RecordNotificationDeliveryAttemptCommand{
		UserNotificationID: 9, UserID: 7, Channel: "email", DeliveryTargetKey: "primary_email", Status: "succeeded",
		AttemptedAt: time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC),
	}
	result, err := service.RecordDeliveryAttempt(context.Background(), command)
	if err != nil || result.AttemptNo != 1 || repository.delivery.UserNotificationID != 9 {
		t.Fatalf("RecordDeliveryAttempt() = %#v/%v, repository = %#v", result, err, repository.delivery)
	}
}

func TestServiceAcceptsWebSocketAsAnIndependentDeliveryChannel(t *testing.T) {
	repository := &notificationRepositoryStub{}
	service, _ := NewService(repository)
	command := RecordNotificationDeliveryAttemptCommand{
		UserNotificationID: 12, UserID: 7, Channel: "websocket", DeliveryTargetKey: "browser_ws", Status: "succeeded",
		AttemptedAt: time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC),
	}
	if _, err := service.RecordDeliveryAttempt(context.Background(), command); err != nil {
		t.Fatalf("RecordDeliveryAttempt(websocket) error = %v", err)
	}
	if repository.delivery.Channel != "websocket" || repository.delivery.DeliveryTargetKey != "browser_ws" {
		t.Fatalf("repository delivery = %#v", repository.delivery)
	}
}
