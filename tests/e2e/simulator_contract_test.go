package e2e_test

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ProviderBehavior enumerates the five simulator behaviors required by V1 E2E.
type ProviderBehavior string

const (
	BehaviorNormal      ProviderBehavior = "normal"
	BehaviorRateLimit   ProviderBehavior = "rate_limit"
	BehaviorAuthInvalid ProviderBehavior = "auth_invalid"
	BehaviorSchemaChange ProviderBehavior = "schema_change"
	BehaviorEmptyResult ProviderBehavior = "empty_result"
)

// AllBehaviors returns all five required simulator behaviors.
func AllBehaviors() []ProviderBehavior {
	return []ProviderBehavior{
		BehaviorNormal,
		BehaviorRateLimit,
		BehaviorAuthInvalid,
		BehaviorSchemaChange,
		BehaviorEmptyResult,
	}
}

// --- AI Provider Simulator Contract ---

// AIResponse represents a simulated AI provider response.
type AIResponse struct {
	Text   string
	Vector []float64
	Model  string
}

// AIProviderSimulator simulates DashScope embedding + chat behaviors.
type AIProviderSimulator interface {
	SetBehavior(b ProviderBehavior)
	Embed(ctx context.Context, text string) ([]float64, error)
	Chat(ctx context.Context, prompt string) (string, error)
	Reset()
}

// --- SMTP Sink Contract ---

// EmailRecord captures a simulated email send.
type EmailRecord struct {
	From    string
	To      string
	Subject string
	Body    string
	SentAt  time.Time
}

// SMTPSink simulates an SMTP server that captures emails.
type SMTPSink interface {
	Addr() string
	Records() []EmailRecord
	Reset()
}

// --- Fetcher Simulator Contract ---

// FetcherSimulator simulates RSS/public page fetcher behaviors.
type FetcherSimulator interface {
	SetBehavior(b ProviderBehavior)
	Fetch(ctx context.Context, sourceURL string) ([]map[string]string, error)
	Reset()
}

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
				assertSimulatorError(t, err, tt.errType)
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
	assertSimulatorError(t, err, "rate_limit")
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
				assertSimulatorError(t, err, tt.errType)
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

// --- Simulator constructors (will fail until implemented) ---

var (
	errNotImplemented = errors.New("simulator not implemented")
)

func newAISimulator(t *testing.T) AIProviderSimulator {
	t.Helper()
	// Will be replaced with real implementation
	return &stubAISimulator{}
}

func newSMTPSink(t *testing.T) SMTPSink {
	t.Helper()
	return &stubSMTPSink{}
}

func newFetcherSimulator(t *testing.T) FetcherSimulator {
	t.Helper()
	return &stubFetcherSimulator{}
}

// --- Stubs that return errors (red phase) ---

type stubAISimulator struct{}

func (s *stubAISimulator) SetBehavior(b ProviderBehavior) {}
func (s *stubAISimulator) Embed(ctx context.Context, text string) ([]float64, error) {
	return nil, errNotImplemented
}
func (s *stubAISimulator) Chat(ctx context.Context, prompt string) (string, error) {
	return "", errNotImplemented
}
func (s *stubAISimulator) Reset() {}

type stubSMTPSink struct{}

func (s *stubSMTPSink) Addr() string           { return "" }
func (s *stubSMTPSink) Records() []EmailRecord  { return nil }
func (s *stubSMTPSink) Reset()                  {}

type stubFetcherSimulator struct{}

func (s *stubFetcherSimulator) SetBehavior(b ProviderBehavior) {}
func (s *stubFetcherSimulator) Fetch(ctx context.Context, sourceURL string) ([]map[string]string, error) {
	return nil, errNotImplemented
}
func (s *stubFetcherSimulator) Reset() {}

// --- Error assertion helper ---

func assertSimulatorError(t *testing.T, err error, expectedType string) {
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

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
