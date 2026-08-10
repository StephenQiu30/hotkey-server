package http

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type notificationServiceStub struct {
	mu         sync.Mutex
	queries    []application.ListUserNotificationsQuery
	items      []application.UserNotificationDTO
	deliveries []application.RecordNotificationDeliveryAttemptCommand
	err        error
}

func (stub *notificationServiceStub) ListUserNotifications(_ context.Context, query application.ListUserNotificationsQuery) (application.ListUserNotificationsResult, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.queries = append(stub.queries, query)
	if stub.err != nil {
		return application.ListUserNotificationsResult{}, stub.err
	}
	page := application.ListUserNotificationsResult{Items: []application.UserNotificationDTO{}, NextAfterID: query.AfterID}
	for _, item := range stub.items {
		if item.ID > query.AfterID && item.UserID == query.UserID {
			page.Items = append(page.Items, item)
			page.NextAfterID = item.ID
		}
	}
	return page, nil
}

func (stub *notificationServiceStub) RecordDeliveryAttempt(_ context.Context, command application.RecordNotificationDeliveryAttemptCommand) (application.RecordNotificationDeliveryAttemptResult, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.deliveries = append(stub.deliveries, command)
	return application.RecordNotificationDeliveryAttemptResult{DeliveryAttemptID: int64(len(stub.deliveries)), AttemptNo: 1}, nil
}

func TestListUsesAuthenticatedUserAndSafeEnvelope(t *testing.T) {
	stub := &notificationServiceStub{items: []application.UserNotificationDTO{notificationFixture(8)}}
	handler := mustNotificationHandler(t, stub, StreamConfig{PollInterval: time.Millisecond, HeartbeatInterval: time.Second, MaxConnections: 2})
	router := gin.New()
	RegisterRoutes(router, handler, notificationAuthenticator{role: httptransport.RoleEditor})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/notifications?after_id=7&limit=20&monitor_id=4", nil)
	request.Header.Set("Authorization", "Bearer fixture")
	router.ServeHTTP(response, request)

	if response.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	stub.mu.Lock()
	query := stub.queries[0]
	stub.mu.Unlock()
	if query.UserID != 1 || query.AfterID != 7 || query.Limit != 20 || query.MonitorID == nil || *query.MonitorID != 4 {
		t.Fatalf("list query = %#v", query)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	var result struct {
		Code int                             `json:"code"`
		Data UserNotificationPageResponseDTO `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 0 || len(result.Data.Items) != 1 || result.Data.NextAfterID != 8 || result.Data.Items[0].DeepLink != "/dashboard/events?event=42" {
		t.Fatalf("result = %#v", result)
	}
}

func TestStreamResumesFromLastEventIDEmitsSafeFrameAndRecordsDelivery(t *testing.T) {
	stub := &notificationServiceStub{items: []application.UserNotificationDTO{notificationFixture(12)}}
	handler := mustNotificationHandler(t, stub, StreamConfig{PollInterval: 5 * time.Millisecond, HeartbeatInterval: time.Second, MaxConnections: 2})
	router := gin.New()
	RegisterRoutes(router, handler, notificationAuthenticator{role: httptransport.RoleViewer})
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, server.URL+"/api/v1/notifications/stream?after_id=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer fixture")
	request.Header.Set("Last-Event-ID", "11")
	response, err := stdhttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != stdhttp.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream response = %d %#v", response.StatusCode, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	frame := ""
	for !strings.Contains(frame, "\n\n") {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatal(readErr)
		}
		frame += line
	}
	if !strings.Contains(frame, "id: 12\n") || !strings.Contains(frame, "event: micro_event.updated\n") ||
		!strings.Contains(frame, `"resource_id":42`) || strings.Contains(frame, "payload") || strings.Contains(frame, "object_key") {
		t.Fatalf("SSE frame = %q", frame)
	}
	cancel()
	stub.mu.Lock()
	query := stub.queries[0]
	deliveries := append([]application.RecordNotificationDeliveryAttemptCommand(nil), stub.deliveries...)
	stub.mu.Unlock()
	if query.AfterID != 11 || query.UserID != 1 {
		t.Fatalf("stream query = %#v", query)
	}
	if len(deliveries) != 1 || deliveries[0].UserNotificationID != 12 || deliveries[0].Channel != "sse" ||
		deliveries[0].DeliveryTargetKey != "browser_stream" || deliveries[0].Status != "succeeded" {
		t.Fatalf("deliveries = %#v", deliveries)
	}
}

func TestStreamRejectsInvalidCursorCapacityAndAnonymousAccess(t *testing.T) {
	stub := &notificationServiceStub{}
	handler := mustNotificationHandler(t, stub, StreamConfig{PollInterval: time.Millisecond, HeartbeatInterval: time.Second, MaxConnections: 1})

	for _, fixture := range []struct {
		name          string
		authenticator httptransport.Authenticator
		path          string
		occupy        bool
		wantStatus    int
	}{
		{name: "invalid cursor", authenticator: notificationAuthenticator{role: httptransport.RoleAdmin}, path: "/api/v1/notifications/stream?after_id=-1", wantStatus: stdhttp.StatusBadRequest},
		{name: "invalid monitor", authenticator: notificationAuthenticator{role: httptransport.RoleAdmin}, path: "/api/v1/notifications/stream?monitor_id=0", wantStatus: stdhttp.StatusBadRequest},
		{name: "capacity", authenticator: notificationAuthenticator{role: httptransport.RoleAdmin}, path: "/api/v1/notifications/stream", occupy: true, wantStatus: stdhttp.StatusServiceUnavailable},
		{name: "anonymous", authenticator: httptransport.NewUnavailableAuthenticator(), path: "/api/v1/notifications/stream", wantStatus: stdhttp.StatusUnauthorized},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if fixture.occupy {
				handler.slots <- struct{}{}
				defer func() { <-handler.slots }()
			}
			router := gin.New()
			RegisterRoutes(router, handler, fixture.authenticator)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(stdhttp.MethodGet, fixture.path, nil)
			request.Header.Set("Authorization", "Bearer fixture")
			router.ServeHTTP(response, request)
			if response.Code != fixture.wantStatus {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("status = %d, want %d, body = %s", response.Code, fixture.wantStatus, body)
			}
		})
	}
}

type notificationAuthenticator struct{ role httptransport.Role }

func (authenticator notificationAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 1, Role: authenticator.role}, nil
}

func notificationFixture(id int64) application.UserNotificationDTO {
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	return application.UserNotificationDTO{
		ID: id, Version: 1, OutboxEventID: id, UserID: 1, MonitorID: 4,
		EventType: "micro_event.updated", ResourceType: "micro_event", ResourceID: 42, ResourceVersion: 3,
		OccurredAt: now, Title: "事件发生重要变化", Summary: "新增独立来源", ResourceStatus: "active",
		DeepLink: "/dashboard/events?event=42", CreatedAt: now,
	}
}

func mustNotificationHandler(t *testing.T, service notificationService, config StreamConfig) *Handler {
	t.Helper()
	handler, err := NewHandler(service, config)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
