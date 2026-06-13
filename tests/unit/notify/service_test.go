package notify_test

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/notify"
	fakenotify "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/notify"
)

func TestListUnreadNotificationsReturnsNewestFirst(t *testing.T) {
	now := time.Now()
	repo := &fakenotify.Repo{
		Notifications: []notify.Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now.Add(-2 * time.Hour)},
			{ID: 2, UserID: 1, AlertID: 11, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now.Add(-1 * time.Hour)},
			{ID: 3, UserID: 1, AlertID: 12, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
		},
	}
	svc := notify.NewService(repo)
	items, err := svc.ListUnread(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// Newest first: ID 3, 2, 1
	if items[0].ID != 3 {
		t.Errorf("expected newest first (ID 3), got ID %d", items[0].ID)
	}
	if items[2].ID != 1 {
		t.Errorf("expected oldest last (ID 1), got ID %d", items[2].ID)
	}
}

func TestListUnreadExcludesReadNotifications(t *testing.T) {
	now := time.Now()
	readAt := now.Add(-30 * time.Minute)
	repo := &fakenotify.Repo{
		Notifications: []notify.Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
			{ID: 2, UserID: 1, AlertID: 11, Channel: "in_app", DeliveryStatus: "sent", ReadAt: &readAt, CreatedAt: now},
		},
	}
	svc := notify.NewService(repo)
	items, err := svc.ListUnread(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 unread item, got %d", len(items))
	}
	if items[0].ID != 1 {
		t.Errorf("expected unread item ID 1, got ID %d", items[0].ID)
	}
}

func TestListUnreadExcludesOtherUsers(t *testing.T) {
	now := time.Now()
	repo := &fakenotify.Repo{
		Notifications: []notify.Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
			{ID: 2, UserID: 2, AlertID: 11, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
		},
	}
	svc := notify.NewService(repo)
	items, err := svc.ListUnread(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item for user 1, got %d", len(items))
	}
}

func TestMarkReadSetsReadAt(t *testing.T) {
	repo := &fakenotify.Repo{
		Notifications: []notify.Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent"},
		},
	}
	svc := notify.NewService(repo)
	err := svc.MarkRead(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.Notifications[0].ReadAt == nil {
		t.Fatal("expected ReadAt to be set")
	}
}

func TestMarkReadRejectsWrongUser(t *testing.T) {
	repo := &fakenotify.Repo{
		Notifications: []notify.Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent"},
		},
	}
	svc := notify.NewService(repo)
	err := svc.MarkRead(context.Background(), 99, 1)
	if err == nil {
		t.Fatal("expected error for wrong user")
	}
}
