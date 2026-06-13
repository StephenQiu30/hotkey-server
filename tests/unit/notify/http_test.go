package notify_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/notify"
	fakenotify "github.com/StephenQiu30/hotkey-server/tests/testutil/fake/notify"
)

func TestListUnreadHandlerReturnsJSON(t *testing.T) {
	now := time.Now()
	repo := &fakenotify.Repo{
		Notifications: []notify.Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent", CreatedAt: now},
		},
	}
	svc := notify.NewService(repo)
	handler := notify.NewHandler(svc)

	req := httptest.NewRequest("GET", "/api/v1/notifications", nil)
	req = req.WithContext(notify.ContextWithUserID(req.Context(), 1))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var items []struct {
		ID             int64  `json:"id"`
		AlertID        int64  `json:"alert_id"`
		Channel        string `json:"channel"`
		DeliveryStatus string `json:"delivery_status"`
	}
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

func TestListUnreadHandlerRequiresAuth(t *testing.T) {
	repo := &fakenotify.Repo{}
	svc := notify.NewService(repo)
	handler := notify.NewHandler(svc)

	req := httptest.NewRequest("GET", "/api/v1/notifications", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMarkReadHandlerReturnsNoContent(t *testing.T) {
	repo := &fakenotify.Repo{
		Notifications: []notify.Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent"},
		},
	}
	svc := notify.NewService(repo)
	handler := notify.NewHandler(svc)

	req := httptest.NewRequest("POST", "/api/v1/notifications/1/read", nil)
	req = req.WithContext(notify.ContextWithUserID(req.Context(), 1))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestMarkReadHandlerReturnsNotFoundForWrongUser(t *testing.T) {
	repo := &fakenotify.Repo{
		Notifications: []notify.Notification{
			{ID: 1, UserID: 1, AlertID: 10, Channel: "in_app", DeliveryStatus: "sent"},
		},
	}
	svc := notify.NewService(repo)
	handler := notify.NewHandler(svc)

	req := httptest.NewRequest("POST", "/api/v1/notifications/1/read", nil)
	req = req.WithContext(notify.ContextWithUserID(req.Context(), 99))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
