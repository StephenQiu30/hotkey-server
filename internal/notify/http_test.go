package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListUnreadHandlerReturnsJSON(t *testing.T) {
	now := time.Now()
	repo := &fakeNotificationRepo{
		notifications: []Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
		},
	}
	svc := NewService(repo)
	handler := NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/notifications?user_id=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var items []notificationJSON
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", items[0].ID)
	}
}

func TestListUnreadHandlerRequiresUserID(t *testing.T) {
	repo := &fakeNotificationRepo{}
	svc := NewService(repo)
	handler := NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/notifications", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMarkReadHandlerReturnsNoContent(t *testing.T) {
	repo := &fakeNotificationRepo{
		notifications: []Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent"},
		},
	}
	svc := NewService(repo)
	handler := NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/v1/notifications/1/read?user_id=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestMarkReadHandlerReturnsNotFoundForWrongUser(t *testing.T) {
	repo := &fakeNotificationRepo{
		notifications: []Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent"},
		},
	}
	svc := NewService(repo)
	handler := NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/v1/notifications/1/read?user_id=99", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
