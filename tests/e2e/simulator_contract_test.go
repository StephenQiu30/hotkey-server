//go:build e2e

package e2e_test

import (
	"bufio"
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

	reader := bufio.NewReader(conn)

	// readSMTPReply reads SMTP reply lines until terminal line for code is reached.
	readSMTPReply := func(expectCode string) string {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var all strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("SMTP read: %v", err)
			}
			all.WriteString(line)
			// Terminal line per RFC SMTP reply framing: "XYZ " (not "XYZ-").
			if strings.HasPrefix(line, expectCode+" ") {
				return all.String()
			}
			if !strings.HasPrefix(line, expectCode+"-") {
				t.Fatalf("unexpected SMTP reply line %q (want prefix %s)", line, expectCode)
			}
		}
	}

	// Minimal SMTP flow — write command and validate server response.
	writeCmd := func(format string, args ...any) {
		t.Helper()
		if _, err := fmt.Fprintf(conn, format, args...); err != nil {
			t.Fatalf("SMTP write %q: %v", fmt.Sprintf(format, args...), err)
		}
	}

	resp := readSMTPReply("220")
	if !strings.HasPrefix(resp, "220") {
		t.Fatalf("expected 220 greeting, got %q", resp)
	}
	writeCmd("EHLO localhost\r\n")
	resp = readSMTPReply("250")
	if !strings.HasPrefix(resp, "250") {
		t.Fatalf("expected 250 for EHLO, got %q", resp)
	}
	writeCmd("MAIL FROM:<sender@example.com>\r\n")
	resp = readSMTPReply("250")
	if !strings.HasPrefix(resp, "250") {
		t.Fatalf("expected 250 for MAIL FROM, got %q", resp)
	}
	writeCmd("RCPT TO:<receiver@example.com>\r\n")
	resp = readSMTPReply("250")
	if !strings.HasPrefix(resp, "250") {
		t.Fatalf("expected 250 for RCPT TO, got %q", resp)
	}
	writeCmd("DATA\r\n")
	resp = readSMTPReply("354")
	if !strings.HasPrefix(resp, "354") {
		t.Fatalf("expected 354 for DATA, got %q", resp)
	}
	writeCmd("Subject: Test Subject\r\n\r\nTest Body\r\n.\r\n")
	resp = readSMTPReply("250")
	if !strings.HasPrefix(resp, "250") {
		t.Fatalf("expected 250 for message body, got %q", resp)
	}
	writeCmd("QUIT\r\n")
	resp = readSMTPReply("221")
	if !strings.HasPrefix(resp, "221") {
		t.Fatalf("expected 221 for QUIT, got %q", resp)
	}

	// Poll until sink processes the email or timeout.
	deadline := time.After(3 * time.Second)
	for {
		if len(sink.Records()) >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for SMTP sink to capture email")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

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
	if !strings.Contains(r.Body, "Test Body") {
		t.Errorf("expected body to contain 'Test Body', got %q", r.Body)
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
	t.Cleanup(func() { _ = sink.Close() })
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
