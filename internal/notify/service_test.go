package notify

import (
	"context"
	"testing"
	"time"
)

func TestListUnreadNotificationsReturnsNewestFirst(t *testing.T) {
	now := time.Now()
	repo := &fakeNotificationRepo{
		notifications: []Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now.Add(-2 * time.Hour)},
			{ID: 2, UserID: 1, AlertID: 11, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now.Add(-1 * time.Hour)},
			{ID: 3, UserID: 1, AlertID: 12, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
		},
	}
	svc := NewService(repo)
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
	repo := &fakeNotificationRepo{
		notifications: []Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
			{ID: 2, UserID: 1, AlertID: 11, Channel: "in_app", DeliveryStatus: "sent", ReadAt: &readAt, CreatedAt: now},
		},
	}
	svc := NewService(repo)
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
	repo := &fakeNotificationRepo{
		notifications: []Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
			{ID: 2, UserID: 2, AlertID: 11, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
		},
	}
	svc := NewService(repo)
	items, err := svc.ListUnread(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item for user 1, got %d", len(items))
	}
}

func TestMarkReadSetsReadAt(t *testing.T) {
	repo := &fakeNotificationRepo{
		notifications: []Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent"},
		},
	}
	svc := NewService(repo)
	err := svc.MarkRead(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.notifications[0].ReadAt == nil {
		t.Fatal("expected ReadAt to be set")
	}
}

func TestMarkReadRejectsWrongUser(t *testing.T) {
	repo := &fakeNotificationRepo{
		notifications: []Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent"},
		},
	}
	svc := NewService(repo)
	err := svc.MarkRead(context.Background(), 99, 1)
	if err == nil {
		t.Fatal("expected error for wrong user")
	}
}

// fakeNotificationRepo is an in-memory implementation of Repository for testing.
type fakeNotificationRepo struct {
	notifications []Notification
}

func (r *fakeNotificationRepo) ListUnread(_ context.Context, userID int64) ([]Notification, error) {
	var result []Notification
	for _, n := range r.notifications {
		if n.UserID == userID && n.ReadAt == nil {
			result = append(result, n)
		}
	}
	// Sort newest first by CreatedAt
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].CreatedAt.After(result[i].CreatedAt) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result, nil
}

func (r *fakeNotificationRepo) MarkRead(_ context.Context, userID, notificationID int64) error {
	for i, n := range r.notifications {
		if n.ID == notificationID {
			if n.UserID != userID {
				return ErrNotOwned
			}
			now := time.Now()
			r.notifications[i].ReadAt = &now
			return nil
		}
	}
	return ErrNotFound
}

func (r *fakeNotificationRepo) Create(_ context.Context, n Notification) (Notification, error) {
	n.ID = int64(len(r.notifications) + 1)
	r.notifications = append(r.notifications, n)
	return n, nil
}
