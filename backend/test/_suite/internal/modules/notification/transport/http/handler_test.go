package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"
)

type notificationServiceStub struct {
	mu               sync.Mutex
	queries          []application.ListUserNotificationsQuery
	items            []application.UserNotificationDTO
	deliveries       []application.RecordNotificationDeliveryAttemptCommand
	deliveryRecorded chan struct{}
	err              error
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
	if stub.deliveryRecorded != nil {
		select {
		case stub.deliveryRecorded <- struct{}{}:
		default:
		}
	}
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

func TestWebSocketAuthenticatesInTheFirstFrameReplaysAndRecordsDelivery(t *testing.T) {
	stub := &notificationServiceStub{items: []application.UserNotificationDTO{notificationFixture(12)}}
	handler := mustNotificationHandler(t, stub, StreamConfig{PollInterval: 20 * time.Millisecond, HeartbeatInterval: time.Second, MaxConnections: 2})
	router := gin.New()
	RegisterRoutes(router, handler, notificationAuthenticator{role: httptransport.RoleViewer})
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, server.URL+"/api/v1/notifications/ws", &websocket.DialOptions{
		Subprotocols: []string{"hotkey.notifications.v1"},
	})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("websocket dial status=%d error=%v", status, err)
	}
	defer connection.CloseNow()
	if connection.Subprotocol() != "hotkey.notifications.v1" {
		t.Fatalf("subprotocol = %q", connection.Subprotocol())
	}
	if err := wsjson.Write(ctx, connection, map[string]any{
		"type": "authenticate", "token": "fixture", "after_id": 11, "monitor_id": 4,
	}); err != nil {
		t.Fatal(err)
	}
	var ready struct {
		Type    string `json:"type"`
		AfterID int64  `json:"after_id"`
	}
	if err := wsjson.Read(ctx, connection, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != "ready" || ready.AfterID != 11 {
		t.Fatalf("ready frame = %#v", ready)
	}
	var frame struct {
		Type  string                      `json:"type"`
		ID    int64                       `json:"id"`
		Event string                      `json:"event"`
		Data  UserNotificationResponseDTO `json:"data"`
	}
	if err := wsjson.Read(ctx, connection, &frame); err != nil {
		t.Fatal(err)
	}
	if frame.Type != "notification" || frame.ID != 12 || frame.Event != "micro_event.updated" ||
		frame.Data.ID != 12 || frame.Data.ResourceID != 42 || frame.Data.DeepLink != "/dashboard/events?event=42" {
		t.Fatalf("notification frame = %#v", frame)
	}

	deadline := time.Now().Add(time.Second)
	for {
		stub.mu.Lock()
		queries := append([]application.ListUserNotificationsQuery(nil), stub.queries...)
		deliveries := append([]application.RecordNotificationDeliveryAttemptCommand(nil), stub.deliveries...)
		stub.mu.Unlock()
		if len(deliveries) > 0 {
			if len(queries) == 0 || queries[0].UserID != 1 || queries[0].AfterID != 11 || queries[0].MonitorID == nil || *queries[0].MonitorID != 4 {
				t.Fatalf("queries = %#v", queries)
			}
			if deliveries[0].Channel != "websocket" || deliveries[0].DeliveryTargetKey != "browser_ws" || deliveries[0].UserNotificationID != 12 {
				t.Fatalf("deliveries = %#v", deliveries)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("websocket delivery was not recorded: %#v", deliveries)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestWebSocketAllowsConfiguredFrontendOriginBehindReverseProxy(t *testing.T) {
	stub := &notificationServiceStub{}
	handler := mustNotificationHandler(t, stub, StreamConfig{
		PollInterval: time.Millisecond, HeartbeatInterval: time.Second, MaxConnections: 2,
		AllowedOrigins: []string{"http://frontend.example.test"},
	})
	router := gin.New()
	RegisterRoutes(router, handler, notificationAuthenticator{role: httptransport.RoleViewer})
	server := httptest.NewServer(router)
	defer server.Close()

	for _, fixture := range []struct {
		name       string
		origin     string
		wantStatus int
	}{
		{name: "configured frontend", origin: "http://frontend.example.test", wantStatus: stdhttp.StatusSwitchingProtocols},
		{name: "untrusted frontend", origin: "http://untrusted.example.test", wantStatus: stdhttp.StatusForbidden},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			connection, response, err := websocket.Dial(ctx, server.URL+"/api/v1/notifications/ws", &websocket.DialOptions{
				Subprotocols: []string{"hotkey.notifications.v1"},
				HTTPHeader:   stdhttp.Header{"Origin": []string{fixture.origin}},
			})
			if connection != nil {
				connection.CloseNow()
			}
			status := stdhttp.StatusSwitchingProtocols
			if response != nil {
				status = response.StatusCode
			}
			if status != fixture.wantStatus {
				t.Fatalf("websocket dial status=%d, want %d, error=%v", status, fixture.wantStatus, err)
			}
		})
	}
}

func TestWebSocketRejectsAuthenticationFramesWithUnknownFields(t *testing.T) {
	stub := &notificationServiceStub{}
	handler := mustNotificationHandler(t, stub, StreamConfig{
		PollInterval: time.Millisecond, HeartbeatInterval: time.Second, MaxConnections: 2,
	})
	router := gin.New()
	RegisterRoutes(router, handler, notificationAuthenticator{role: httptransport.RoleViewer})
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, server.URL+"/api/v1/notifications/ws", &websocket.DialOptions{
		Subprotocols: []string{"hotkey.notifications.v1"},
	})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("websocket dial status=%d error=%v", status, err)
	}
	defer connection.CloseNow()
	if err := wsjson.Write(ctx, connection, map[string]any{
		"type": "authenticate", "token": "fixture", "after_id": 0, "injected": true,
	}); err != nil {
		t.Fatal(err)
	}
	var frame notificationWebSocketControlFrame
	err = wsjson.Read(ctx, connection, &frame)
	if err == nil {
		t.Fatalf("unknown authentication field was accepted: %#v", frame)
	}
	if status := websocket.CloseStatus(err); status != websocket.StatusPolicyViolation {
		t.Fatalf("close status = %d, want %d: %v", status, websocket.StatusPolicyViolation, err)
	}
}

func TestWebSocketClosesWhenClientSendsBusinessFramesAfterAuthentication(t *testing.T) {
	stub := &notificationServiceStub{}
	handler := mustNotificationHandler(t, stub, StreamConfig{
		PollInterval: time.Second, HeartbeatInterval: time.Second, MaxConnections: 2,
	})
	router := gin.New()
	RegisterRoutes(router, handler, notificationAuthenticator{role: httptransport.RoleViewer})
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, server.URL+"/api/v1/notifications/ws", &websocket.DialOptions{
		Subprotocols: []string{"hotkey.notifications.v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if err := wsjson.Write(ctx, connection, map[string]any{
		"type": "authenticate", "token": "fixture", "after_id": 0,
	}); err != nil {
		t.Fatal(err)
	}
	var ready notificationWebSocketControlFrame
	if err := wsjson.Read(ctx, connection, &ready); err != nil || ready.Type != "ready" {
		t.Fatalf("ready frame = %#v / %v", ready, err)
	}
	if err := wsjson.Write(ctx, connection, map[string]any{"type": "inject_notification"}); err != nil {
		t.Fatal(err)
	}
	var response notificationWebSocketControlFrame
	err = wsjson.Read(ctx, connection, &response)
	if status := websocket.CloseStatus(err); status != websocket.StatusPolicyViolation {
		t.Fatalf("close status = %d, want %d: %v", status, websocket.StatusPolicyViolation, err)
	}
}

func TestWebSocketEmitsHeartbeatAndRejectsConnectionsAboveCapacity(t *testing.T) {
	stub := &notificationServiceStub{}
	handler := mustNotificationHandler(t, stub, StreamConfig{
		PollInterval: 100 * time.Millisecond, HeartbeatInterval: 5 * time.Millisecond, MaxConnections: 1,
	})
	router := gin.New()
	RegisterRoutes(router, handler, notificationAuthenticator{role: httptransport.RoleViewer})
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, server.URL+"/api/v1/notifications/ws", &websocket.DialOptions{
		Subprotocols: []string{"hotkey.notifications.v1"},
	})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("first websocket dial status=%d error=%v", status, err)
	}
	defer connection.CloseNow()
	if err := wsjson.Write(ctx, connection, map[string]any{
		"type": "authenticate", "token": "fixture", "after_id": 7,
	}); err != nil {
		t.Fatal(err)
	}
	var ready notificationWebSocketControlFrame
	if err := wsjson.Read(ctx, connection, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != "ready" || ready.AfterID != 7 {
		t.Fatalf("ready frame = %#v", ready)
	}
	var heartbeat notificationWebSocketControlFrame
	if err := wsjson.Read(ctx, connection, &heartbeat); err != nil {
		t.Fatal(err)
	}
	if heartbeat.Type != "heartbeat" || heartbeat.AfterID != 7 || heartbeat.SentAt.IsZero() {
		t.Fatalf("heartbeat frame = %#v", heartbeat)
	}

	overCapacity, capacityResponse, capacityErr := websocket.Dial(ctx, server.URL+"/api/v1/notifications/ws", &websocket.DialOptions{
		Subprotocols: []string{"hotkey.notifications.v1"},
	})
	if overCapacity != nil {
		overCapacity.CloseNow()
	}
	if capacityErr == nil || capacityResponse == nil || capacityResponse.StatusCode != stdhttp.StatusServiceUnavailable {
		status := 0
		if capacityResponse != nil {
			status = capacityResponse.StatusCode
		}
		t.Fatalf("over-capacity websocket status=%d error=%v", status, capacityErr)
	}
}

func TestNotificationWebSocketWriteTimeoutIsBounded(t *testing.T) {
	if notificationWebSocketWriteTimeout <= 0 || notificationWebSocketWriteTimeout > 10*time.Second {
		t.Fatalf("notificationWebSocketWriteTimeout = %s", notificationWebSocketWriteTimeout)
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
