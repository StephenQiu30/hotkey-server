package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// --- Stub repositories for testing ---

type stubAuthRepo struct{ users []auth.User }

func (r *stubAuthRepo) ExistsByEmail(_ context.Context, email string) bool {
	for _, u := range r.users {
		if u.Email == email {
			return true
		}
	}
	return false
}
func (r *stubAuthRepo) Create(_ context.Context, email, passwordHash, displayName string) (auth.User, error) {
	u := auth.User{ID: int64(len(r.users) + 1), Email: email, PasswordHash: passwordHash, DisplayName: displayName, Status: "active", PlanType: "free"}
	r.users = append(r.users, u)
	return u, nil
}
func (r *stubAuthRepo) GetByEmail(_ context.Context, email string) (*auth.User, error) {
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}
func (r *stubAuthRepo) GetByID(_ context.Context, _ int64) (*auth.User, error) { return nil, nil }

type stubMonitorRepo struct{}

func (r *stubMonitorRepo) Create(_ context.Context, _ int64, _ monitor.CreateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{ID: 1, UserID: 1, Name: "test", Status: "active"}, nil
}
func (r *stubMonitorRepo) GetByID(_ context.Context, _ int64) (*monitor.Monitor, error) {
	return nil, monitor.ErrNotFound
}
func (r *stubMonitorRepo) ListByUser(_ context.Context, _ int64) ([]monitor.Monitor, error) {
	return nil, nil
}
func (r *stubMonitorRepo) Update(_ context.Context, _ int64, _ monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, monitor.ErrNotFound
}

type stubNotifyRepo struct{}

func (r *stubNotifyRepo) ListUnread(_ context.Context, _ int64) ([]notify.Notification, error) {
	return nil, nil
}
func (r *stubNotifyRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (r *stubNotifyRepo) Create(_ context.Context, n notify.Notification) (notify.Notification, error) {
	return n, nil
}

type stubPostQueryService struct{}

func (s *stubPostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

type stubTopicQueryService struct{}

func (s *stubTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

type stubTrendQueryService struct{}

func (s *stubTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
func (s *stubTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}

// newTestHandler creates an http.Handler with smoke test mode enabled.
func newTestHandler() http.Handler {
	_, mux := NewAPI(Config{
		JWTSecret:     "test-secret",
		SmokeTest:     true,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})
	return mux
}

func TestHealthEndpoint(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"status"`) {
		t.Fatalf("expected status field in response, got: %s", rr.Body.String())
	}
}

func TestRegisterReturns201(t *testing.T) {
	handler := newTestHandler()
	body := `{"email":"test@example.com","password":"Passw0rd!","display_name":"Test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestLoginReturns200(t *testing.T) {
	handler := newTestHandler()
	// Register first
	regBody := `{"email":"login@example.com","password":"Passw0rd!","display_name":"Login"}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regRR := httptest.NewRecorder()
	handler.ServeHTTP(regRR, regReq)

	if regRR.Code != http.StatusCreated {
		t.Fatalf("register failed: %d", regRR.Code)
	}

	// Login
	loginBody := `{"email":"login@example.com","password":"Passw0rd!"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	handler.ServeHTTP(loginRR, loginReq)

	if loginRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", loginRR.Code, loginRR.Body.String())
	}
	if !strings.Contains(loginRR.Body.String(), `"token"`) {
		t.Fatalf("expected token in response, got: %s", loginRR.Body.String())
	}
}

func TestMonitorsRequireAuth(t *testing.T) {
	// Without smoke test mode, protected endpoints should return 401.
	_, mux := NewAPI(Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/monitors"},
		{http.MethodGet, "/api/v1/monitors/1/posts"},
		{http.MethodGet, "/api/v1/monitors/1/topics"},
		{http.MethodGet, "/api/v1/monitors/1/trends"},
		{http.MethodGet, "/api/v1/topics/1/trends"},
		{http.MethodGet, "/api/v1/notifications"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestSmokeBypassAuth(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list monitors", http.MethodGet, "/api/v1/monitors"},
		{"list posts", http.MethodGet, "/api/v1/monitors/1/posts"},
		{"list topics", http.MethodGet, "/api/v1/monitors/1/topics"},
		{"monitor trends", http.MethodGet, "/api/v1/monitors/1/trends"},
		{"topic trends", http.MethodGet, "/api/v1/topics/1/trends"},
		{"list notifications", http.MethodGet, "/api/v1/notifications"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}
