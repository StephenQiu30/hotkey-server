package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	alertapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type alertServiceFake struct {
	page        domain.ThreadPage
	detail      domain.ThreadDetail
	listQuery   domain.ListQuery
	actions     []alertapplication.ActionInput
	listErr     error
	getErr      error
	actionErr   error
	suppressErr error
}

func (fake *alertServiceFake) List(_ context.Context, query domain.ListQuery) (domain.ThreadPage, error) {
	fake.listQuery = query
	return fake.page, fake.listErr
}

func (fake *alertServiceFake) Get(context.Context, int64) (domain.ThreadDetail, error) {
	return fake.detail, fake.getErr
}

func (fake *alertServiceFake) Acknowledge(_ context.Context, input alertapplication.ActionInput) (domain.Thread, error) {
	fake.actions = append(fake.actions, input)
	return fake.detail.Thread, fake.actionErr
}

func (fake *alertServiceFake) Resolve(_ context.Context, input alertapplication.ActionInput) (domain.Thread, error) {
	fake.actions = append(fake.actions, input)
	return fake.detail.Thread, fake.actionErr
}

func (fake *alertServiceFake) Suppress(_ context.Context, input alertapplication.ActionInput) (domain.Thread, error) {
	fake.actions = append(fake.actions, input)
	return fake.detail.Thread, fake.suppressErr
}

type alertAuthenticator struct{ role httptransport.Role }

func (auth alertAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 7, SessionID: 8, Role: auth.role}, nil
}

func TestAlertRoutesExposeListDetailAndVersionedActionsInResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 8, 4, 6, 0, 0, 0, time.UTC)
	thread := domain.Thread{ID: 9, Version: 3, MonitorID: 10, EventID: 20, TriggerType: domain.TriggerRising, State: domain.StateOpen, Severity: domain.SeverityWarning, TitleSnapshot: "Safe title", ReasonSnapshot: "Safe reason", FirstTriggeredAt: now, LastTriggeredAt: now, OccurrenceCount: 1, CooldownUntil: now.Add(time.Hour)}
	service := &alertServiceFake{
		page:   domain.ThreadPage{Items: []domain.Thread{thread}},
		detail: domain.ThreadDetail{Thread: thread, Occurrences: []domain.Occurrence{{ID: 30, ThreadID: 9, EventUpdateID: 40, Severity: domain.SeverityWarning, FinalScoreSnapshot: 80, Fingerprint: strings.Repeat("f", 64), TriggeredAt: now}}, Audits: []domain.StateAudit{}},
	}
	router := gin.New()
	RegisterRoutes(router, service, alertAuthenticator{role: httptransport.RoleViewer})

	list := alertRequest(router, http.MethodGet, "/api/v1/alerts?state=open&severity=warning&monitor_id=10&limit=25", "", true)
	assertAlertResult(t, list, http.StatusOK, 0, false)
	if service.listQuery.Limit != 25 || service.listQuery.MonitorID == nil || *service.listQuery.MonitorID != 10 || service.listQuery.State == nil || *service.listQuery.State != domain.StateOpen {
		t.Fatalf("list query = %#v", service.listQuery)
	}

	detail := alertRequest(router, http.MethodGet, "/api/v1/alerts/9", "", true)
	assertAlertResult(t, detail, http.StatusOK, 0, false)
	if !strings.Contains(detail.Body.String(), `"title":"Safe title"`) || !strings.Contains(detail.Body.String(), `"occurrences"`) {
		t.Fatalf("detail body = %s", detail.Body.String())
	}
	for _, forbidden := range []string{"fingerprint", "queue_payload", "provider_error", "event_body", "credential_ref"} {
		if strings.Contains(detail.Body.String(), forbidden) {
			t.Fatalf("detail leaked %q: %s", forbidden, detail.Body.String())
		}
	}

	resolved := alertRequest(router, http.MethodPost, "/api/v1/alerts/9/resolve", `{"expected_version":3,"reason_code":"handled"}`, true)
	assertAlertResult(t, resolved, http.StatusOK, 0, false)
	if len(service.actions) != 1 || service.actions[0].ThreadID != 9 || service.actions[0].ExpectedVersion != 3 || service.actions[0].ReasonCode != "handled" || service.actions[0].Subject.UserID != 7 {
		t.Fatalf("action input = %#v", service.actions)
	}
}

func TestViewerMayAcknowledgeButMayNotSuppress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &alertServiceFake{detail: domain.ThreadDetail{Thread: domain.Thread{ID: 9, Version: 2, State: domain.StateOpen}}}
	router := gin.New()
	RegisterRoutes(router, service, alertAuthenticator{role: httptransport.RoleViewer})

	acknowledged := alertRequest(router, http.MethodPost, "/api/v1/alerts/9/acknowledge", `{"expected_version":2,"reason_code":"seen"}`, true)
	assertAlertResult(t, acknowledged, http.StatusOK, 0, false)
	suppressed := alertRequest(router, http.MethodPost, "/api/v1/alerts/9/suppress", `{"expected_version":2,"reason_code":"irrelevant"}`, true)
	assertAlertResult(t, suppressed, http.StatusForbidden, 20001, true)
	if len(service.actions) != 1 {
		t.Fatalf("viewer suppress reached service; actions = %#v", service.actions)
	}
}

func TestAlertRoutesMapStableErrorsAndNeverLeakCauses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		path       string
		method     string
		body       string
		configure  func(*alertServiceFake)
		wantStatus int
		wantCode   int
	}{
		{name: "invalid id", path: "/api/v1/alerts/0", method: http.MethodGet, wantStatus: http.StatusBadRequest, wantCode: 10000},
		{name: "not found", path: "/api/v1/alerts/99", method: http.MethodGet, configure: func(fake *alertServiceFake) { fake.getErr = sharedrepository.ErrNotFound }, wantStatus: http.StatusNotFound, wantCode: 10003},
		{name: "stale version", path: "/api/v1/alerts/9/resolve", method: http.MethodPost, body: `{"expected_version":1,"reason_code":"done"}`, configure: func(fake *alertServiceFake) { fake.actionErr = sharedrepository.ErrConflict }, wantStatus: http.StatusConflict, wantCode: 10002},
		{name: "unavailable", path: "/api/v1/alerts", method: http.MethodGet, configure: func(fake *alertServiceFake) { fake.listErr = sharedrepository.ErrUnavailable }, wantStatus: http.StatusServiceUnavailable, wantCode: 90001},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &alertServiceFake{}
			if test.configure != nil {
				test.configure(service)
			}
			router := gin.New()
			RegisterRoutes(router, service, alertAuthenticator{role: httptransport.RoleAdmin})
			response := alertRequest(router, test.method, test.path, test.body, true)
			assertAlertResult(t, response, test.wantStatus, test.wantCode, true)
			if strings.Contains(response.Body.String(), "repository") || strings.Contains(response.Body.String(), "SELECT") {
				t.Fatalf("error leaked an internal cause: %s", response.Body.String())
			}
		})
	}
}

func alertRequest(router *gin.Engine, method, path, body string, authenticated bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.Header.Set("Authorization", "Bearer test-token")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertAlertResult(t *testing.T, response *httptest.ResponseRecorder, wantStatus, wantCode int, wantNullData bool) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode Result: %v body=%s", err, response.Body.String())
	}
	if len(result) != 3 || result["code"] == nil || result["message"] == nil || result["data"] == nil {
		t.Fatalf("Result keys = %#v", result)
	}
	var code int
	if err := json.Unmarshal(result["code"], &code); err != nil || code != wantCode {
		t.Fatalf("business code = %d/%v, want %d", code, err, wantCode)
	}
	if wantNullData && string(result["data"]) != "null" {
		t.Fatalf("error data = %s, want null", result["data"])
	}
}
