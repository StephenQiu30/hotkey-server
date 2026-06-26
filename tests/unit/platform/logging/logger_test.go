package logging_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	platformlogging "github.com/StephenQiu30/hotkey-server/internal/platform/logging"
)

func TestLoggerWritesStructuredFields(t *testing.T) {
	var out bytes.Buffer
	logger := platformlogging.New(&out)

	logger.Event(platformlogging.Event{
		RequestID:  "req-log",
		TraceID:    "trace-log",
		UserID:     42,
		Module:     "monitor",
		Action:     "create",
		Err:        errors.New("boom"),
		DurationMS: 17,
		Time:       time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
	})

	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON log, got %v: %s", err, out.String())
	}

	assertStringField(t, body, "request_id", "req-log")
	assertStringField(t, body, "trace_id", "trace-log")
	assertStringField(t, body, "module", "monitor")
	assertStringField(t, body, "action", "create")
	assertStringField(t, body, "error", "boom")
	assertStringField(t, body, "time", "2026-06-26T12:00:00Z")

	if got := body["user_id"]; got != float64(42) {
		t.Fatalf("expected user_id 42, got %#v", got)
	}
	if got := body["duration_ms"]; got != float64(17) {
		t.Fatalf("expected duration_ms 17, got %#v", got)
	}
}

func assertStringField(t *testing.T, body map[string]any, key, want string) {
	t.Helper()
	if got := body[key]; got != want {
		t.Fatalf("expected %s=%q, got %#v", key, want, got)
	}
}
