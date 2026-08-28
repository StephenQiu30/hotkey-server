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
	project  ProjectUserNotificationCommand
	receipt  RecordNotificationReadReceiptCommand
	read     RecordNotificationReadReceiptResult
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

func (stub *notificationRepositoryStub) ProjectUserNotification(_ context.Context, command ProjectUserNotificationCommand) (ProjectUserNotificationResult, error) {
	stub.project = command
	return ProjectUserNotificationResult{UserNotificationID: 13, Created: true}, stub.err
}

func (stub *notificationRepositoryStub) RecordNotificationReadReceipt(_ context.Context, command RecordNotificationReadReceiptCommand) (RecordNotificationReadReceiptResult, error) {
	stub.receipt = command
	return stub.read, stub.err
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

func TestServiceRejectsNotificationOutsideRequestedMonitor(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	requestedMonitorID := int64(4)
	repository := &notificationRepositoryStub{page: ListUserNotificationsResult{
		Items: []UserNotificationDTO{{
			ID: 9, Version: 1, OutboxEventID: 11, UserID: 7, MonitorID: 5,
			EventType: "micro_event.updated", ResourceType: "micro_event", ResourceID: 42, ResourceVersion: 2,
			OccurredAt: now, Title: "越界 Monitor 通知", ResourceStatus: "active",
			DeepLink: "/dashboard/events?event=42", CreatedAt: now,
		}},
		NextAfterID: 9,
	}}
	service, _ := NewService(repository)

	if _, err := service.ListUserNotifications(context.Background(), ListUserNotificationsQuery{
		UserID: 7, MonitorID: &requestedMonitorID, Limit: 10,
	}); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("ListUserNotifications(cross-monitor result) error = %v, want constraint", err)
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

func TestServiceProjectsOnlyAnExactVersionedOutboxFact(t *testing.T) {
	repository := &notificationRepositoryStub{}
	service, _ := NewService(repository)
	result, err := service.ProjectUserNotification(context.Background(), ProjectUserNotificationCommand{
		OutboxEventID: 11, OutboxVersion: 1,
	})
	if err != nil || result.UserNotificationID != 13 || !result.Created {
		t.Fatalf("ProjectUserNotification() = %#v/%v", result, err)
	}
	if repository.project.OutboxEventID != 11 || repository.project.OutboxVersion != 1 {
		t.Fatalf("repository project command = %#v", repository.project)
	}
	if _, err := service.ProjectUserNotification(context.Background(), ProjectUserNotificationCommand{OutboxEventID: 11}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("ProjectUserNotification(invalid version) error = %v", err)
	}
}

func TestServiceRecordsOnlyCurrentUsersMonotonicReadReceipt(t *testing.T) {
	recordedAt := time.Date(2026, 8, 12, 2, 0, 0, 0, time.UTC)
	repository := &notificationRepositoryStub{read: RecordNotificationReadReceiptResult{
		ReceiptID: 3, ReadThroughID: 12, Advanced: true, RecordedAt: recordedAt,
	}}
	service, _ := NewService(repository)
	result, err := service.RecordNotificationReadReceipt(context.Background(), RecordNotificationReadReceiptCommand{
		UserID: 7, ReadThroughID: 12,
	})
	if err != nil || result.ReceiptID != 3 || result.ReadThroughID != 12 || !result.Advanced {
		t.Fatalf("RecordNotificationReadReceipt() = %#v/%v", result, err)
	}
	if repository.receipt.UserID != 7 || repository.receipt.ReadThroughID != 12 {
		t.Fatalf("repository receipt = %#v", repository.receipt)
	}
	if _, err := service.RecordNotificationReadReceipt(context.Background(), RecordNotificationReadReceiptCommand{
		UserID: 0, ReadThroughID: 12,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("RecordNotificationReadReceipt(invalid user) error = %v", err)
	}
}
