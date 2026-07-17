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
	metrics.RecordContentQuery("list_active", "success")
	request := httptest.NewRequest("GET", "/metrics", nil)
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "hotkey_collection_operations_total") || !strings.Contains(response.Body.String(), "hotkey_content_query_operations_total") {
		t.Fatalf("/metrics counters = status %d body %q", response.Code, response.Body.String())
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
		"hotkey_content_query_operations_total",
	} {
		if !names[name] {
			t.Errorf("missing metric family %q", name)
		}
	}
}

func TestContentQueryMetricUsesOnlyOperationAndOutcomeLabels(t *testing.T) {
	t.Parallel()

	metrics, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	metrics.RecordContentQuery("get_active", "not_found")
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather(): %v", err)
	}
	for _, family := range families {
		if family.GetName() != "hotkey_content_query_operations_total" {
			continue
		}
		if len(family.Metric) != 1 {
			t.Fatalf("content query metric count = %d, want 1", len(family.Metric))
		}
		labels := map[string]string{}
		for _, label := range family.Metric[0].Label {
			labels[label.GetName()] = label.GetValue()
		}
		if len(labels) != 2 || labels["operation"] != "get_active" || labels["outcome"] != "not_found" {
			t.Fatalf("content query metric labels = %#v, want only operation/outcome", labels)
		}
		return
	}
	t.Fatal("content query metric family is missing")
}
