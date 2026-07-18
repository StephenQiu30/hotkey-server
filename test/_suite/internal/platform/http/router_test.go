package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestHealthEndpointsUseResultContract(t *testing.T) {
	t.Parallel()

	router, telemetry := newRouterForTest(t, ReadinessFunc(func(context.Context) error { return nil }))
	defer func() { _ = telemetry.Shutdown(context.Background()) }()

	for _, path := range []string{"/healthz", "/readyz"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			request := httptest.NewRequest(stdhttp.MethodGet, path, nil)
			router.ServeHTTP(response, request)

			if response.Code != stdhttp.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusOK)
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("X-Request-ID header is empty")
			}
			assertResult(t, response, 0, "success", map[string]any{"status": "ok"})
		})
	}
}

func TestReadyDoesNotExposeInternalFailure(t *testing.T) {
	t.Parallel()

	router, telemetry := newRouterForTest(t, ReadinessFunc(func(context.Context) error {
		return errors.New("postgres password=secret is unavailable")
	}))
	defer func() { _ = telemetry.Shutdown(context.Background()) }()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/readyz", nil)

	router.ServeHTTP(response, request)

	if response.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusServiceUnavailable)
	}
	assertResult(t, response, 90001, "service not ready", nil)
}

func TestAPIDocumentationIsAvailableOutsideProduction(t *testing.T) {
	t.Parallel()

	router, telemetry := newRouterForTest(t, ReadinessFunc(func(context.Context) error { return nil }))
	defer func() { _ = telemetry.Shutdown(context.Background()) }()

	tests := []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/openapi.json", contentType: "application/json", contains: `"swagger": "2.0"`},
		{path: "/docs/index.html", contentType: "text/html", contains: "Swagger UI"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			request := httptest.NewRequest(stdhttp.MethodGet, test.path, nil)
			router.ServeHTTP(response, request)

			if response.Code != stdhttp.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusOK)
			}
			if got := response.Header().Get("Content-Type"); !strings.Contains(got, test.contentType) {
				t.Errorf("Content-Type = %q, want it to contain %q", got, test.contentType)
			}
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(body), test.contains) {
				t.Errorf("body does not contain %q", test.contains)
			}
		})
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/docs", nil)
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusTemporaryRedirect {
		t.Fatalf("/docs status = %d, want %d", response.Code, stdhttp.StatusTemporaryRedirect)
	}
	if location := response.Header().Get("Location"); location != "/docs/index.html" {
		t.Errorf("/docs Location = %q, want %q", location, "/docs/index.html")
	}
}

func TestAPIDocumentationIsDisabledInProduction(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Environment = "production"
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	telemetry, err := observability.NewTelemetry(cfg)
	if err != nil {
		t.Fatalf("NewTelemetry() error = %v", err)
	}
	defer func() { _ = telemetry.Shutdown(context.Background()) }()
	router := NewRouter(
		ReadinessFunc(func(context.Context) error { return nil }),
		metrics,
		telemetry,
		zap.NewNop(),
		cfg,
	)

	for _, path := range []string{"/openapi.json", "/docs/index.html"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(stdhttp.MethodGet, path, nil)
		router.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusNotFound {
			t.Errorf("%s status = %d, want %d", path, response.Code, stdhttp.StatusNotFound)
		}
	}
}

func assertResult(t *testing.T, response *httptest.ResponseRecorder, code int, message string, data any) {
	t.Helper()

	var got Result[any]
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Code != code {
		t.Errorf("code = %d, want %d", got.Code, code)
	}
	if got.Message != message {
		t.Errorf("message = %q, want %q", got.Message, message)
	}
	if data == nil {
		if got.Data != nil {
			t.Errorf("data = %#v, want nil", got.Data)
		}
		return
	}
	want, _ := json.Marshal(data)
	actual, _ := json.Marshal(got.Data)
	if string(actual) != string(want) {
		t.Errorf("data = %s, want %s", actual, want)
	}
}

func newRouterForTest(t *testing.T, readiness Readiness) (*gin.Engine, *observability.Telemetry) {
	t.Helper()
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	telemetry, err := observability.NewTelemetry(config.Default())
	if err != nil {
		t.Fatalf("NewTelemetry() error = %v", err)
	}
	return NewRouter(readiness, metrics, telemetry, zap.NewNop(), config.Default()), telemetry
}
