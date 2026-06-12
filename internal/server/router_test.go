package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stephenqiu/hotkey-server/internal/auth"
	"github.com/stephenqiu/hotkey-server/internal/monitor"
)

func newTestRouter() *http.ServeMux {
	authRepo := &fakeAuthRepo{}
	authSvc := auth.NewService(authRepo, "test-secret")
	authHandler := auth.NewHTTPHandler(authSvc)

	monitorRepo := &fakeMonitorRepo{}
	monitorSvc := monitor.NewService(monitorRepo)
	monitorHandler := monitor.NewHTTPHandler(monitorSvc)

	return NewRouter(Dependencies{
		AuthHandler:    authHandler,
		MonitorHandler: monitorHandler,
		JWTSecret:      "test-secret",
	})
}

func TestProtectedMonitorRoutesRequireAuth(t *testing.T) {
	router := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestHealthRouteStillWorks(t *testing.T) {
	router := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// Test doubles for router tests
type fakeAuthRepo struct{}

func (r *fakeAuthRepo) ExistsByEmail(_ context.Context, email string) bool { return false }
func (r *fakeAuthRepo) Create(_ context.Context, email, passwordHash, displayName string) (auth.User, error) {
	return auth.User{ID: 1, Email: email, DisplayName: displayName, Status: "active", PlanType: "free"}, nil
}
func (r *fakeAuthRepo) FindByEmail(_ context.Context, email string) (auth.User, string, error) {
	return auth.User{}, "", nil
}

type fakeMonitorRepo struct{}

func (r *fakeMonitorRepo) Create(_ context.Context, userID int64, input monitor.CreateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{ID: 1, UserID: userID, Name: input.Name, Status: "active"}, nil
}
func (r *fakeMonitorRepo) ListByUser(_ context.Context, userID int64) ([]monitor.Monitor, error) {
	return nil, nil
}
func (r *fakeMonitorRepo) GetByID(_ context.Context, id int64) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}
func (r *fakeMonitorRepo) Update(_ context.Context, id int64, input monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}
func (r *fakeMonitorRepo) Deactivate(_ context.Context, id int64) error { return nil }
