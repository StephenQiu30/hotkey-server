package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsUseDedicatedRegistry(t *testing.T) {
	t.Parallel()

	metrics, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	metrics.RecordHTTPRequest("GET", "/api/v1/capabilities", 200, 25*time.Millisecond)
	metrics.RecordPanic("/panic")
	metrics.SetDependencyHealth("database", 1)
	metrics.RecordCollectionOperation("retry", "success")
	request := httptest.NewRequest("GET", "/metrics", nil)
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "hotkey_collection_operations_total") {
		t.Fatalf("/metrics collection counter = status %d body %q", response.Code, response.Body.String())
	}

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	names := make(map[string]bool, len(families))
	for _, family := range families {
		names[family.GetName()] = true
	}
	for _, name := range []string{
		"hotkey_http_requests_total",
		"hotkey_http_request_duration_seconds",
		"hotkey_http_panics_total",
		"hotkey_dependency_health",
		"hotkey_collection_operations_total",
	} {
		if !names[name] {
			t.Errorf("missing metric family %q", name)
		}
	}
}
