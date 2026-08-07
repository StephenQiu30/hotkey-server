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
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type notificationServiceStub struct {
	mu     sync.Mutex
	inputs []application.ListInput
	events []domain.NotificationEvent
	err    error
}

func (stub *notificationServiceStub) ListAfter(_ context.Context, input application.ListInput) (domain.NotificationPage, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.inputs = append(stub.inputs, input)
	if stub.err != nil {
		return domain.NotificationPage{}, stub.err
	}
	page := domain.NotificationPage{Items: []domain.NotificationEvent{}, NextAfterID: input.AfterID}
	for _, event := range stub.events {
		if event.ID > input.AfterID && event.Audience.Allows(event.Audience) {
			page.Items = append(page.Items, event)
			page.NextAfterID = event.ID
		}
	}
	return page, nil
}

func TestListUsesAuthenticatedRoleAndStableEnvelope(t *testing.T) {
	stub := &notificationServiceStub{events: []domain.NotificationEvent{notificationFixture(8)}}
	handler := mustNotificationHandler(t, stub, StreamConfig{PollInterval: time.Millisecond, HeartbeatInterval: time.Second, MaxConnections: 2})
	router := gin.New()
	RegisterRoutes(router, handler, notificationAuthenticator{role: httptransport.RoleEditor})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/notifications?after_id=7&limit=20", nil)
	request.Header.Set("Authorization", "Bearer fixture")
	router.ServeHTTP(response, request)

	if response.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	stub.mu.Lock()
	input := stub.inputs[0]
	stub.mu.Unlock()
	if input.Role != domain.AudienceEditor || input.AfterID != 7 || input.Limit != 20 {
		t.Fatalf("list input = %#v", input)
	}
	var result struct {
		Code int                      `json:"code"`
		Data NotificationPageResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 0 || len(result.Data.Items) != 1 || result.Data.NextAfterID != 8 {
		t.Fatalf("result = %#v", result)
	}
}

func TestStreamResumesFromLastEventIDAndEmitsSSEFrame(t *testing.T) {
	stub := &notificationServiceStub{events: []domain.NotificationEvent{notificationFixture(12)}}
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
	if !strings.Contains(frame, "id: 12\n") || !strings.Contains(frame, "event: event.updated\n") || !strings.Contains(frame, `"resource_id":42`) {
		t.Fatalf("SSE frame = %q", frame)
	}
	cancel()
	stub.mu.Lock()
	input := stub.inputs[0]
	stub.mu.Unlock()
	if input.AfterID != 11 || input.Role != domain.AudienceViewer {
		t.Fatalf("stream input = %#v", input)
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

func notificationFixture(id int64) domain.NotificationEvent {
	return domain.NotificationEvent{
		ID: id, EventType: domain.EventUpdated, ResourceType: domain.ResourceEvent, ResourceID: 42,
		Audience: domain.AudienceViewer, OccurredAt: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC),
		Payload: domain.NotificationPayload{Title: "事件发生重要变化", Summary: "热度上升", Status: "rising"},
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
