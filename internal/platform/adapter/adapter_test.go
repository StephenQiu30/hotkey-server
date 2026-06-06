package adapter_test

import (
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
)

// --- AdapterError contract tests ---

func TestAdapterErrorWrapsFailureClass(t *testing.T) {
	err := adapter.NewAdapterError(adapter.FailureClassAuth, "token expired", nil)
	if err.Class != adapter.FailureClassAuth {
		t.Fatalf("expected class %q, got %q", adapter.FailureClassAuth, err.Class)
	}
	if err.Error() != "token expired" {
		t.Fatalf("expected message %q, got %q", "token expired", err.Error())
	}
}

func TestAdapterErrorWrapsCause(t *testing.T) {
	cause := errors.New("underlying")
	err := adapter.NewAdapterError(adapter.FailureClassTransient, "timeout", cause)
	if !errors.Is(err, cause) {
		t.Fatalf("expected error to wrap cause")
	}
	if err.Error() != "timeout: underlying" {
		t.Fatalf("expected wrapped message, got %q", err.Error())
	}
}

func TestIsAdapterErrorMatchesClass(t *testing.T) {
	err := adapter.NewAdapterError(adapter.FailureClassRateLimit, "429", nil)
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatal("expected IsAdapterError to match rate_limit class")
	}
	if adapter.IsAdapterError(err, adapter.FailureClassAuth) {
		t.Fatal("expected IsAdapterError to not match auth class")
	}
}

func TestIsAdapterErrorReturnsFalseForNonAdapterError(t *testing.T) {
	err := errors.New("plain error")
	if adapter.IsAdapterError(err, adapter.FailureClassTransient) {
		t.Fatal("expected false for non-AdapterError")
	}
}

// --- Simulator contract tests ---

func TestSimulatorReturnsNormalizedItems(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "test-rss",
		Items: []adapter.NormalizedItem{
			{Title: "Article 1", URL: "https://example.com/1", Snippet: "snippet 1", ExternalID: "ext-1", PublishedAt: &now, Language: "en"},
			{Title: "Article 2", URL: "https://example.com/2", Snippet: "snippet 2", ExternalID: "ext-2", PublishedAt: &now, Language: "zh"},
		},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-1",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(output.Items))
	}
	if output.Items[0].Title != "Article 1" {
		t.Fatalf("expected title %q, got %q", "Article 1", output.Items[0].Title)
	}
	if output.Items[1].Language != "zh" {
		t.Fatalf("expected language %q, got %q", "zh", output.Items[1].Language)
	}
}

func TestSimulatorReportsHealth(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "test-rss",
	})

	health := sim.Health()
	if health.Status != adapter.HealthStatusHealthy {
		t.Fatalf("expected healthy status, got %q", health.Status)
	}
	if health.LastCheckedAt.IsZero() {
		t.Fatal("expected LastCheckedAt to be set")
	}
}

func TestSimulatorReportsCapabilities(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "test-rss",
		Capabilities: adapter.Capabilities{
			SupportsIncremental: true,
			MaxItemsPerFetch:    50,
			RateLimitPerHour:    100,
		},
	})

	caps := sim.Capabilities()
	if !caps.SupportsIncremental {
		t.Fatal("expected SupportsIncremental to be true")
	}
	if caps.MaxItemsPerFetch != 50 {
		t.Fatalf("expected MaxItemsPerFetch 50, got %d", caps.MaxItemsPerFetch)
	}
	if caps.RateLimitPerHour != 100 {
		t.Fatalf("expected RateLimitPerHour 100, got %d", caps.RateLimitPerHour)
	}
}

func TestSimulatorReturnsAuthError(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider:  adapter.ProviderOfficialAPI,
		Name:      "test-api",
		CollectErr: adapter.NewAdapterError(adapter.FailureClassAuth, "unauthorized", nil),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-1",
		Provider: adapter.ProviderOfficialAPI,
		URL:      "https://api.example.com",
	})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassAuth) {
		t.Fatalf("expected auth failure class, got %v", err)
	}
}

func TestSimulatorReturnsRateLimitError(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider:  adapter.ProviderOfficialAPI,
		Name:      "test-api",
		CollectErr: adapter.NewAdapterError(adapter.FailureClassRateLimit, "rate limited", nil),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-1",
		Provider: adapter.ProviderOfficialAPI,
		URL:      "https://api.example.com",
	})
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatalf("expected rate_limit failure class, got %v", err)
	}
}

func TestSimulatorHealthDegradedAfterError(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider:  adapter.ProviderRSS,
		Name:      "test-rss",
		CollectErr: adapter.NewAdapterError(adapter.FailureClassTransient, "timeout", nil),
	})

	_, _ = sim.Collect(adapter.CollectInput{
		SourceID: "src-1",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss",
	})

	health := sim.Health()
	if health.Status != adapter.HealthStatusDegraded {
		t.Fatalf("expected degraded after error, got %q", health.Status)
	}
	if health.LastError == "" {
		t.Fatal("expected LastError to be set after failure")
	}
}

func TestSimulatorReturnsEmptyResult(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "test-rss",
		Items:    []adapter.NormalizedItem{},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-1",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(output.Items))
	}
}

func TestSimulatorRespectsIdempotencyKey(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "test-rss",
		Items: []adapter.NormalizedItem{
			{Title: "Article 1", URL: "https://example.com/1", ExternalID: "ext-1", PublishedAt: &now},
		},
	})

	input := adapter.CollectInput{
		SourceID:       "src-1",
		Provider:       adapter.ProviderRSS,
		URL:            "https://example.com/rss",
		IdempotencyKey: "idem-key-1",
	}

	output1, err := sim.Collect(input)
	if err != nil {
		t.Fatalf("first collect: %v", err)
	}
	output2, err := sim.Collect(input)
	if err != nil {
		t.Fatalf("second collect: %v", err)
	}

	// Same idempotency key should return same results
	if len(output1.Items) != len(output2.Items) {
		t.Fatalf("expected same item count for same idempotency key")
	}
}

func TestSimulatorHealthRecoversAfterSuccess(t *testing.T) {
	callCount := 0
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "test-rss",
		CollectFn: func(input adapter.CollectInput) (adapter.CollectOutput, error) {
			callCount++
			if callCount == 1 {
				return adapter.CollectOutput{}, adapter.NewAdapterError(adapter.FailureClassTransient, "timeout", nil)
			}
			return adapter.CollectOutput{Items: []adapter.NormalizedItem{{Title: "ok"}}}, nil
		},
	})

	// First call fails
	_, _ = sim.Collect(adapter.CollectInput{SourceID: "src-1", Provider: adapter.ProviderRSS, URL: "https://example.com/rss"})
	if sim.Health().Status != adapter.HealthStatusDegraded {
		t.Fatal("expected degraded after failure")
	}

	// Second call succeeds - health should recover
	_, _ = sim.Collect(adapter.CollectInput{SourceID: "src-1", Provider: adapter.ProviderRSS, URL: "https://example.com/rss"})
	if sim.Health().Status != adapter.HealthStatusHealthy {
		t.Fatalf("expected healthy after success, got %q", sim.Health().Status)
	}
}

// --- Registry contract tests ---

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := adapter.NewRegistry()
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-sim",
	})
	reg.Register(sim)

	got, ok := reg.Get(adapter.ProviderRSS)
	if !ok {
		t.Fatal("expected to find rss adapter")
	}
	if got.Name() != "rss-sim" {
		t.Fatalf("expected name %q, got %q", "rss-sim", got.Name())
	}
}

func TestRegistryGetReturnsFalseForMissing(t *testing.T) {
	reg := adapter.NewRegistry()
	_, ok := reg.Get(adapter.ProviderOfficialAPI)
	if ok {
		t.Fatal("expected false for missing adapter")
	}
}

func TestRegistryListAllAdapters(t *testing.T) {
	reg := adapter.NewRegistry()
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{Provider: adapter.ProviderRSS, Name: "rss"}))
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{Provider: adapter.ProviderPublicPage, Name: "page"}))

	adapters := reg.List()
	if len(adapters) != 2 {
		t.Fatalf("expected 2 adapters, got %d", len(adapters))
	}
}

func TestRegistryOverwriteOnDuplicateProvider(t *testing.T) {
	reg := adapter.NewRegistry()
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{Provider: adapter.ProviderRSS, Name: "old"}))
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{Provider: adapter.ProviderRSS, Name: "new"}))

	got, _ := reg.Get(adapter.ProviderRSS)
	if got.Name() != "new" {
		t.Fatalf("expected overwritten name %q, got %q", "new", got.Name())
	}
}

// --- Idempotency key generation ---

func TestNewIdempotencyKeyIsUnique(t *testing.T) {
	k1 := adapter.NewIdempotencyKey("src-1", "https://example.com/1")
	k2 := adapter.NewIdempotencyKey("src-1", "https://example.com/1")
	if k1 == "" || k2 == "" {
		t.Fatal("expected non-empty idempotency keys")
	}
	// Deterministic for same input
	if k1 != k2 {
		t.Fatalf("expected deterministic keys for same input, got %q vs %q", k1, k2)
	}
}

func TestNewIdempotencyKeyDiffersForDifferentInput(t *testing.T) {
	k1 := adapter.NewIdempotencyKey("src-1", "https://example.com/1")
	k2 := adapter.NewIdempotencyKey("src-1", "https://example.com/2")
	if k1 == k2 {
		t.Fatal("expected different keys for different URLs")
	}
}
