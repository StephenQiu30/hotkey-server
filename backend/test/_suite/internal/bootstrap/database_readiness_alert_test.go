package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	"go.uber.org/zap"
)

type readinessPingerFake struct {
	err error
}

func (fake *readinessPingerFake) Ping(context.Context) error {
	return fake.err
}

func TestDatabaseReadinessProjectsExternalAlertSignalAndClearsAfterRecovery(t *testing.T) {
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := observability.NewTelemetry(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = telemetry.Shutdown(context.Background()) }()

	pinger := &readinessPingerFake{err: errors.New("postgres password=must-not-leak is unavailable")}
	readiness := newDatabaseReadiness(
		httptransport.ReadinessFunc(func(context.Context) error { return nil }),
		pinger,
		metrics,
	)
	router := httptransport.NewRouter(readiness, metrics, telemetry, zap.NewNop(), config.Default())

	for attempt := 1; attempt <= 3; attempt++ {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "must-not-leak") {
			t.Fatalf("failed readiness attempt %d = status %d body %q", attempt, response.Code, response.Body.String())
		}
		if attempt < 3 {
			assertDatabaseAlertMetric(t, metrics, "0")
		}
	}
	assertDatabaseHealthMetric(t, metrics, "0")
	assertDatabaseAlertMetric(t, metrics, "1")

	pinger.err = nil
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("recovered readiness = status %d body %q", response.Code, response.Body.String())
	}
	assertDatabaseHealthMetric(t, metrics, "1")
	assertDatabaseAlertMetric(t, metrics, "0")
}

func assertDatabaseHealthMetric(t *testing.T, metrics *observability.Metrics, expected string) {
	t.Helper()
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	needle := `hotkey_dependency_health{dependency="database"} ` + expected
	if response.Code != http.StatusOK || !strings.Contains(body, needle) || strings.Contains(body, "must-not-leak") {
		t.Fatalf("database health metric = status %d body %q", response.Code, body)
	}
}

func assertDatabaseAlertMetric(t *testing.T, metrics *observability.Metrics, expected string) {
	t.Helper()
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	needle := `hotkey_operational_alert{alert_id="ALERT-DB-UNAVAILABLE",owner="hotkey-oncall",policy_version="p0-operational-alerts-v1",severity="p0",silence_key="ALERT-DB-UNAVAILABLE"} ` + expected
	if response.Code != http.StatusOK || !strings.Contains(body, needle) || strings.Contains(body, "must-not-leak") {
		t.Fatalf("database alert metric = status %d body %q", response.Code, body)
	}
}
