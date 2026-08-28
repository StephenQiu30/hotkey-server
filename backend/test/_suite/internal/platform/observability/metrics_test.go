package observability

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type metricCardinalityFixture struct {
	Version         string   `json:"version"`
	UniqueValues    int      `json:"unique_values"`
	LargeValueBytes int      `json:"large_value_bytes"`
	Classes         []string `json:"classes"`
}

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

func TestMetricsCollapseUntrustedValuesToBoundedLabels(t *testing.T) {
	fixture := loadMetricCardinalityFixture(t)
	metrics, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	largeValue := strings.Repeat("x", fixture.LargeValueBytes)
	for index := 0; index < fixture.UniqueValues; index++ {
		canary := fmt.Sprintf("observability-secret-%04d@sensitive.example", index)
		untrusted := largeValue + canary
		metrics.RecordHTTPRequest("METHOD-"+canary, "https://sensitive.example/"+untrusted, 600+index, time.Millisecond)
		metrics.RecordPanic("/" + untrusted)
		metrics.SetDependencyHealth(untrusted, 1)
		metrics.RecordCollectionOperation(untrusted, canary)
		metrics.RecordContentQuery(untrusted, canary)
	}
	metrics.RecordHTTPRequest("GET", "/api/v1/users/:id", 200, time.Millisecond)
	metrics.RecordPanic("/api/v1/users/:id")
	metrics.SetDependencyHealth("database", 1)
	metrics.RecordCollectionOperation("retry", "success")
	metrics.RecordContentQuery("get_active", "not_found")

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if response.Code != 200 {
		t.Fatalf("metrics status = %d", response.Code)
	}
	body := response.Body.String()
	if strings.Contains(body, "sensitive.example") || strings.Contains(body, largeValue) {
		t.Fatalf("metric exposition leaked untrusted values")
	}

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, family := range families {
		if got := len(family.Metric); got > 2 {
			t.Fatalf("metric family %s has %d series after %d unique inputs, want at most 2", family.GetName(), got, fixture.UniqueValues)
		}
	}
	for _, want := range []string{
		`method="OTHER"`, `route="unmatched"`, `status="other"`,
		`dependency="unknown"`, `operation="unknown"`, `outcome="unknown"`,
		`route="/api/v1/users"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metric exposition is missing bounded label %s", want)
		}
	}
}

func loadMetricCardinalityFixture(t *testing.T) metricCardinalityFixture {
	t.Helper()
	encoded, err := os.ReadFile("../../../test/fixtures/security/observability-cardinality.json")
	if err != nil {
		t.Fatalf("read cardinality fixture: %v", err)
	}
	var fixture metricCardinalityFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("decode cardinality fixture: %v", err)
	}
	if fixture.Version != "observability-cardinality-v1" || fixture.UniqueValues < 1000 || fixture.LargeValueBytes < 4096 || len(fixture.Classes) != 7 {
		t.Fatalf("invalid cardinality fixture: %#v", fixture)
	}
	return fixture
}
