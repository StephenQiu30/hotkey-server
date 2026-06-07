package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
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

			text, err := sim.Chat(ctx, "生成日报")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("behavior %s: expected error, got text: %s", tt.behavior, text)
				}
				assertSimulatorErrorf(t, err, tt.errType)
			} else {
				if err != nil {
					t.Fatalf("behavior %s: unexpected error: %v", tt.behavior, err)
				}
				if text == "" {
					t.Fatalf("behavior %s: expected non-empty response", tt.behavior)
				}
			}
		})
	}
}

// TestSMTPSink_Capture verifies the SMTP sink captures sent emails.
func TestSMTPSink_Capture(t *testing.T) {
	sink := newSMTPSink(t)
	addr := sink.Addr()
	if addr == "" {
		t.Fatal("SMTP sink returned empty address")
	}

	// Send a test email
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to connect to SMTP sink: %v", err)
	}
	defer conn.Close()

	// Minimal SMTP flow
	fmt.Fprintf(conn, "EHLO localhost\r\n")
	fmt.Fprintf(conn, "MAIL FROM:<sender@example.com>\r\n")
	fmt.Fprintf(conn, "RCPT TO:<receiver@example.com>\r\n")
	fmt.Fprintf(conn, "DATA\r\n")
	fmt.Fprintf(conn, "Subject: Test Subject\r\n\r\nTest Body\r\n.\r\n")
	fmt.Fprintf(conn, "QUIT\r\n")

	// Wait a bit for sink to process
	time.Sleep(100 * time.Millisecond)

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("SMTP sink expected 1 record, got %d", len(records))
	}

	r := records[0]
	if r.From != "sender@example.com" {
		t.Errorf("expected from sender@example.com, got %s", r.From)
	}
	if r.To != "receiver@example.com" {
		t.Errorf("expected to receiver@example.com, got %s", r.To)
	}
	if r.Subject != "Test Subject" {
		t.Errorf("expected subject 'Test Subject', got %q", r.Subject)
	}
	if r.Body == "" {
		t.Error("expected non-empty body")
	}
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
	if !strings.Contains(err.Error(), expectedType) {
		t.Fatalf("expected error containing %q, got: %v", expectedType, err)
	}
}
