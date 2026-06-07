package e2e_test

import (
	"context"
	"errors"
	"testing"
)

// --- Contract Tests ---

// TestAISimulator_AllBehaviors verifies the AI simulator supports all five behaviors.
func TestAISimulator_AllBehaviors(t *testing.T) {
	sim := newAISimulator(t)
	ctx := context.Background()

	tests := []struct {
		behavior ProviderBehavior
		wantErr  bool
		errType  string
	}{
		{BehaviorNormal, false, ""},
		{BehaviorRateLimit, true, "rate_limit"},
		{BehaviorAuthInvalid, true, "auth_invalid"},
		{BehaviorSchemaChange, true, "schema_change"},
		{BehaviorEmptyResult, true, "empty_result"},
	}

	for _, tt := range tests {
		t.Run(string(tt.behavior), func(t *testing.T) {
			sim.SetBehavior(tt.behavior)
			defer sim.Reset()

			vec, err := sim.Embed(ctx, "test input")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("behavior %s: expected error, got vector len=%d", tt.behavior, len(vec))
				}
				assertSimulatorErrorf(t, err, tt.errType)
			} else {
				if err != nil {
					t.Fatalf("behavior %s: unexpected error: %v", tt.behavior, err)
				}
				if len(vec) == 0 {
					t.Fatalf("behavior %s: expected non-empty vector", tt.behavior)
				}
			}
		})
	}
}

// TestAISimulator_Chat verifies chat simulation for all behaviors.
func TestAISimulator_Chat(t *testing.T) {
	sim := newAISimulator(t)
	ctx := context.Background()

	sim.SetBehavior(BehaviorNormal)
	text, err := sim.Chat(ctx, "生成日报")
	if err != nil {
		t.Fatalf("normal chat: unexpected error: %v", err)
	}
	if text == "" {
		t.Fatal("normal chat: expected non-empty response")
	}

	sim.SetBehavior(BehaviorRateLimit)
	_, err = sim.Chat(ctx, "生成日报")
	if err == nil {
		t.Fatal("rate_limit chat: expected error")
	}
	assertSimulatorErrorf(t, err, "rate_limit")
}

// TestSMTPSink_Capture verifies the SMTP sink captures sent emails.
func TestSMTPSink_Capture(t *testing.T) {
	sink := newSMTPSink(t)
	addr := sink.Addr()
	if addr == "" {
		t.Fatal("SMTP sink returned empty address")
	}

	records := sink.Records()
	if len(records) != 0 {
		t.Fatalf("SMTP sink expected 0 records initially, got %d", len(records))
	}
	t.Logf("SMTP sink listening at %s", addr)
}

// TestFetcherSimulator_AllBehaviors verifies the fetcher simulator supports all five behaviors.
func TestFetcherSimulator_AllBehaviors(t *testing.T) {
	sim := newFetcherSimulator(t)
	ctx := context.Background()

	tests := []struct {
		behavior ProviderBehavior
		wantErr  bool
		errType  string
	}{
		{BehaviorNormal, false, ""},
		{BehaviorRateLimit, true, "rate_limit"},
		{BehaviorAuthInvalid, true, "auth_invalid"},
		{BehaviorSchemaChange, true, "schema_change"},
		{BehaviorEmptyResult, true, "empty_result"},
	}

	for _, tt := range tests {
		t.Run(string(tt.behavior), func(t *testing.T) {
			sim.SetBehavior(tt.behavior)
			defer sim.Reset()

			items, err := sim.Fetch(ctx, "https://example.com/feed.xml")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("behavior %s: expected error, got %d items", tt.behavior, len(items))
				}
				assertSimulatorErrorf(t, err, tt.errType)
			} else {
				if err != nil {
					t.Fatalf("behavior %s: unexpected error: %v", tt.behavior, err)
				}
				if len(items) == 0 {
					t.Fatalf("behavior %s: expected non-empty items", tt.behavior)
				}
			}
		})
	}
}

// TestAllBehaviors_Coverage verifies all five behaviors are defined.
func TestAllBehaviors_Coverage(t *testing.T) {
	behaviors := AllBehaviors()
	if len(behaviors) != 5 {
		t.Fatalf("expected 5 behaviors, got %d", len(behaviors))
	}
	seen := map[ProviderBehavior]bool{}
	for _, b := range behaviors {
		seen[b] = true
	}
	for _, expected := range []ProviderBehavior{
		BehaviorNormal, BehaviorRateLimit, BehaviorAuthInvalid,
		BehaviorSchemaChange, BehaviorEmptyResult,
	} {
		if !seen[expected] {
			t.Errorf("missing behavior: %s", expected)
		}
	}
}

// --- Simulator constructors (wired to real implementations) ---

func newAISimulator(t *testing.T) AIProviderSimulator {
	t.Helper()
	return newAISimulatorImpl()
}

func newSMTPSink(t *testing.T) SMTPSink {
	t.Helper()
	sink, err := newSMTPSinkImpl()
	if err != nil {
		t.Fatalf("create SMTP sink: %v", err)
	}
	return sink
}

func newFetcherSimulator(t *testing.T) FetcherSimulator {
	t.Helper()
	return newFetcherSimulatorImpl()
}

// --- Error assertion helper (test-only, calls shared helper) ---

func assertSimulatorErrorf(t *testing.T, err error, expectedType string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error, got nil", expectedType)
	}
	var simErr *SimulatorError
	if errors.As(err, &simErr) {
		if simErr.Code != expectedType {
			t.Fatalf("expected error code %q, got %q", expectedType, simErr.Code)
		}
		return
	}
	// Fallback: check error message contains expected type
	if !containsSubstring(err.Error(), expectedType) {
		t.Fatalf("expected error containing %q, got: %v", expectedType, err)
	}
}
