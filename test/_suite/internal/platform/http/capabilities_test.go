package http

import (
	"context"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapabilitiesRoute(t *testing.T) {
	router, telemetry := newRouterForTest(t, ReadinessFunc(func(context.Context) error { return nil }))
	defer func() { _ = telemetry.Shutdown(context.Background()) }()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodGet, "/api/v1/capabilities", nil))

	if response.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusOK)
	}
	assertResult(t, response, 0, "success", map[string]any{"api_version": "v1"})
}

func TestOperationalEndpointsKeepTheirOwnContracts(t *testing.T) {
	router, telemetry := newRouterForTest(t, ReadinessFunc(func(context.Context) error { return nil }))
	defer func() { _ = telemetry.Shutdown(context.Background()) }()

	for _, path := range []string{"/healthz", "/readyz"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodGet, path, nil))
		if response.Code != stdhttp.StatusOK {
			t.Errorf("%s status = %d, want %d", path, response.Code, stdhttp.StatusOK)
		}
		assertResult(t, response, 0, "success", map[string]any{"status": "ok"})
	}

	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, httptest.NewRequest(stdhttp.MethodGet, "/metrics", nil))
	if metricsResponse.Code != stdhttp.StatusOK {
		t.Fatalf("/metrics status = %d, want %d", metricsResponse.Code, stdhttp.StatusOK)
	}
	if contentType := metricsResponse.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("/metrics Content-Type = %q, want Prometheus text", contentType)
	}
}
