package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMiddlewarePreservesOrCreatesRequestIDAndRedactsLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	router, _, telemetry := newTestRouter(t, zap.New(core), config.Default())
	defer func() { _ = telemetry.Shutdown(context.Background()) }()
	router.GET("/ok", Wrap(func(c *gin.Context) error {
		OK(c, map[string]string{"status": "ok"})
		return nil
	}))

	request := httptest.NewRequest(stdhttp.MethodGet, "/ok", nil)
	request.Header.Set("X-Request-ID", "request-42")
	request.Header.Set("Authorization", "Bearer top-secret")
	request.Header.Set("Cookie", "session=top-secret")
	request.Header.Set("Set-Cookie", "session=top-secret")
	request.Header.Set("X-API-Key", "top-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get("X-Request-ID"); got != "request-42" {
		t.Errorf("X-Request-ID = %q, want request-42", got)
	}
	createdResponse := httptest.NewRecorder()
	router.ServeHTTP(createdResponse, httptest.NewRequest(stdhttp.MethodGet, "/ok", nil))
	if _, err := uuid.Parse(createdResponse.Header().Get("X-Request-ID")); err != nil {
		t.Errorf("generated X-Request-ID is not a UUID: %v", err)
	}

	for _, entry := range logs.All() {
		encoded, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatalf("marshal log fields: %v", err)
		}
		if strings.Contains(string(encoded), "top-secret") {
			t.Fatalf("log leaked secret: %s", encoded)
		}
		if got := entry.ContextMap()["module"]; got != "platform" {
			t.Errorf("log module = %#v, want platform", got)
		}
	}
}

func TestMiddlewareMapsPanicAndDeadline(t *testing.T) {
	cfg := config.Default()
	cfg.RequestTimeout = time.Millisecond
	router, metrics, telemetry := newTestRouter(t, zap.NewNop(), cfg)
	defer func() { _ = telemetry.Shutdown(context.Background()) }()
	router.GET("/panic", Wrap(func(*gin.Context) error { panic("postgres password=secret") }))
	router.GET("/deadline", Wrap(func(c *gin.Context) error {
		<-c.Request.Context().Done()
		return c.Request.Context().Err()
	}))

	panicResponse := httptest.NewRecorder()
	router.ServeHTTP(panicResponse, httptest.NewRequest(stdhttp.MethodGet, "/panic", nil))
	if panicResponse.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("panic status = %d, want %d", panicResponse.Code, stdhttp.StatusInternalServerError)
	}
	assertResult(t, panicResponse, sharederrors.CodeInternal, "internal server error", nil)
	if strings.Contains(panicResponse.Body.String(), "secret") {
		t.Fatal("panic response leaked secret")
	}

	deadlineResponse := httptest.NewRecorder()
	router.ServeHTTP(deadlineResponse, httptest.NewRequest(stdhttp.MethodGet, "/deadline", nil))
	if deadlineResponse.Code != stdhttp.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, want %d", deadlineResponse.Code, stdhttp.StatusGatewayTimeout)
	}
	assertResult(t, deadlineResponse, sharederrors.CodeDeadlineExceeded, "deadline exceeded", nil)

	if got := metricNames(t, metrics); !got["hotkey_http_panics_total"] {
		t.Fatal("panic metric was not recorded")
	}
}

func TestTraceContextExtractsInboundParentAndSetsRequestID(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	telemetry := &observability.Telemetry{TracerProvider: provider}
	defer func() { _ = telemetry.Shutdown(context.Background()) }()
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	router := NewRouter(ReadinessFunc(func(context.Context) error { return nil }), metrics, telemetry, zap.NewNop(), config.Default())
	router.GET("/trace", Wrap(func(c *gin.Context) error {
		OK[any](c, nil)
		return nil
	}))

	requestID := "trace-request"
	request := httptest.NewRequest(stdhttp.MethodGet, "/trace", nil)
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	router.ServeHTTP(httptest.NewRecorder(), request)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	if got := spans[0].Parent.SpanID().String(); got != "00f067aa0ba902b7" {
		t.Errorf("parent span ID = %s, want 00f067aa0ba902b7", got)
	}
	if !hasAttribute(spans[0].Attributes, "http.request_id", requestID) {
		t.Fatal("span is missing http.request_id")
	}
}

func newTestRouter(t *testing.T, logger *zap.Logger, cfg config.Config) (*gin.Engine, *observability.Metrics, *observability.Telemetry) {
	t.Helper()
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	telemetry, err := observability.NewTelemetry(cfg)
	if err != nil {
		t.Fatalf("NewTelemetry() error = %v", err)
	}
	return NewRouter(ReadinessFunc(func(context.Context) error { return nil }), metrics, telemetry, logger, cfg), metrics, telemetry
}

func metricNames(t *testing.T, metrics *observability.Metrics) map[string]bool {
	t.Helper()
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	names := make(map[string]bool, len(families))
	for _, family := range families {
		names[family.GetName()] = true
	}
	return names
}

func hasAttribute(attributes []attribute.KeyValue, key, want string) bool {
	for _, candidate := range attributes {
		if string(candidate.Key) == key && candidate.Value.AsString() == want {
			return true
		}
	}
	return false
}
