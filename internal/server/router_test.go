package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != `{"status":"ok"}` {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestProtectedMonitorRoutesRequireAuth(t *testing.T) {
	// Without auth middleware, monitor routes should 404 (not mounted).
	// With auth middleware that rejects, they should 401.
	rejectMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		})
	}

	monitorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	deps := Dependencies{
		MonitorHandler: monitorHandler,
		AuthMiddleware: rejectMiddleware,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil)
	rr := httptest.NewRecorder()
	NewRouter(deps).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
