package logging

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type cardinalityFixture struct {
	Version         string   `json:"version"`
	UniqueValues    int      `json:"unique_values"`
	LargeValueBytes int      `json:"large_value_bytes"`
	Classes         []string `json:"classes"`
}

func TestSafeCoreKeepsFixedLogSchemaUnderAdversarialCardinality(t *testing.T) {
	fixture := loadCardinalityFixture(t)
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(newSafeCore(core))
	largeValue := strings.Repeat("x", fixture.LargeValueBytes)

	for index := 0; index < fixture.UniqueValues; index++ {
		canary := fmt.Sprintf("observability-secret-%04d@sensitive.example", index)
		logger.Info("HTTP request completed",
			zap.String("request_id", fmt.Sprintf("request-%04d", index)),
			zap.String("trace_id", "4bf92f3577b34da6a3ce929d0e0e4736"),
			zap.String("module", "identity"),
			zap.String("method", "GET"),
			zap.String("route", fmt.Sprintf("/api/v1/users/%d?email=%s", index, canary)),
			zap.Int("status", 200),
			zap.Duration("duration", time.Millisecond),
			zap.String("url", "https://sensitive.example/"+canary),
			zap.String("email", canary),
			zap.String("body", largeValue+canary),
			zap.String("prompt", largeValue+canary),
			zap.String("payload", largeValue+canary),
			zap.String(fmt.Sprintf("dynamic_%04d", index), canary),
			zap.Error(errors.New(canary)),
		)
	}

	if got := logs.Len(); got != fixture.UniqueValues {
		t.Fatalf("log count = %d, want %d", got, fixture.UniqueValues)
	}
	allowedKeys := map[string]bool{
		"request_id": true, "trace_id": true, "module": true, "method": true,
		"route": true, "status": true, "duration": true, "failure_code": true,
	}
	for _, entry := range logs.All() {
		if entry.Message != "HTTP request completed" {
			t.Fatalf("message = %q, want stable application event", entry.Message)
		}
		fields := entry.ContextMap()
		for key := range fields {
			if !allowedKeys[key] {
				t.Fatalf("unexpected searchable log field %q in %#v", key, fields)
			}
		}
		if fields["route"] != "/api/v1/users" || fields["failure_code"] != "internal_error" {
			t.Fatalf("sanitized fields = %#v", fields)
		}
		encoded, err := json.Marshal(fields)
		if err != nil {
			t.Fatalf("marshal log fields: %v", err)
		}
		if strings.Contains(string(encoded), "sensitive.example") || strings.Contains(string(encoded), largeValue) {
			t.Fatalf("log leaked a high-cardinality or sensitive value: %s", encoded)
		}
	}
}

func TestSafeCoreHashesStacksAndRejectsUnapprovedMessages(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(newSafeCore(core))
	secret := "postgres://operator:secret@private.example/hotkey"

	logger.Error("HTTP panic recovered", zap.ByteString("stack", []byte(secret)))
	logger.Info("user message "+secret, zap.String("payload", secret))

	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("log count = %d, want 2", len(entries))
	}
	stackDigest, ok := entries[0].ContextMap()["stack_sha256"].(string)
	if !ok || len(stackDigest) != 64 || strings.Contains(stackDigest, secret) {
		t.Fatalf("stack digest = %#v, want 64-character digest", stackDigest)
	}
	if entries[1].Message != rejectedMessage {
		t.Fatalf("unapproved message = %q, want %q", entries[1].Message, rejectedMessage)
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal log entries: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("logs leaked secret: %s", encoded)
	}
}

func loadCardinalityFixture(t *testing.T) cardinalityFixture {
	t.Helper()
	encoded, err := os.ReadFile("../../../test/fixtures/security/observability-cardinality.json")
	if err != nil {
		t.Fatalf("read cardinality fixture: %v", err)
	}
	var fixture cardinalityFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("decode cardinality fixture: %v", err)
	}
	if fixture.Version != "observability-cardinality-v1" || fixture.UniqueValues < 1000 || fixture.LargeValueBytes < 4096 || len(fixture.Classes) != 7 {
		t.Fatalf("invalid cardinality fixture: %#v", fixture)
	}
	return fixture
}
