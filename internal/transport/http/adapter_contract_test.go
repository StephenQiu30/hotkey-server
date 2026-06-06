package http_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestListAdaptersRequiresAdminAuth(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "adapter-user@example.com")

	denied := getWithBearer(router, "/api/v1/admin/adapters", userToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d with body %s", denied.Code, denied.Body.String())
	}
}

func TestListAdaptersReturnsAllRegisteredAdapters(t *testing.T) {
	reg := adapter.NewRegistry()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-adapter",
		Items: []adapter.NormalizedItem{
			{Title: "Test", URL: "https://example.com/1", PublishedAt: &now},
		},
	}))
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderPublicPage,
		Name:     "page-adapter",
	}))

	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdapterRegistry: reg,
	})
	adminToken := registerAdminAndLogin(t, router, "adapter-admin@example.com")

	adapters := getWithBearer(router, "/api/v1/admin/adapters", adminToken)
	if adapters.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", adapters.Code, adapters.Body.String())
	}
	if adapters.Body.Len() == 0 {
		t.Fatal("expected non-empty adapters list")
	}
}

func TestGetAdapterHealthReturnsStatus(t *testing.T) {
	reg := adapter.NewRegistry()
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-adapter",
	}))

	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdapterRegistry: reg,
	})
	adminToken := registerAdminAndLogin(t, router, "adapter-health@example.com")

	health := getWithBearer(router, "/api/v1/admin/adapters/rss/health", adminToken)
	if health.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", health.Code, health.Body.String())
	}
	assertJSONField(t, health.Body.Bytes(), "status", "healthy")
}

func TestGetAdapterHealthReturns404ForMissing(t *testing.T) {
	reg := adapter.NewRegistry()
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdapterRegistry: reg,
	})
	adminToken := registerAdminAndLogin(t, router, "adapter-missing@example.com")

	health := getWithBearer(router, "/api/v1/admin/adapters/official_api/health", adminToken)
	if health.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d with body %s", health.Code, health.Body.String())
	}
}

func TestGetAdapterCapabilitiesReturnsConfig(t *testing.T) {
	reg := adapter.NewRegistry()
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-adapter",
		Capabilities: adapter.Capabilities{
			SupportsIncremental: true,
			MaxItemsPerFetch:    50,
			RateLimitPerHour:    100,
		},
	}))

	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdapterRegistry: reg,
	})
	adminToken := registerAdminAndLogin(t, router, "adapter-caps@example.com")

	caps := getWithBearer(router, "/api/v1/admin/adapters/rss/capabilities", adminToken)
	if caps.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", caps.Code, caps.Body.String())
	}
	assertJSONField(t, caps.Body.Bytes(), "supports_incremental", "true")
	assertJSONField(t, caps.Body.Bytes(), "max_items_per_fetch", "50")
	assertJSONField(t, caps.Body.Bytes(), "rate_limit_per_hour", "100")
}

func TestGetAdapterCapabilitiesReturns404ForMissing(t *testing.T) {
	reg := adapter.NewRegistry()
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdapterRegistry: reg,
	})
	adminToken := registerAdminAndLogin(t, router, "adapter-caps-miss@example.com")

	caps := getWithBearer(router, "/api/v1/admin/adapters/official_api/capabilities", adminToken)
	if caps.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d with body %s", caps.Code, caps.Body.String())
	}
}
