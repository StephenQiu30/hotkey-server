package observability_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/observability"
)

func TestLoggerIncludesServiceField(t *testing.T) {
	line := observability.RenderLog("api", "started")
	if !strings.Contains(line, `"service":"api"`) {
		t.Fatalf("expected service field in %s", line)
	}
}

func TestLoggerIncludesMessageField(t *testing.T) {
	line := observability.RenderLog("worker", "processing task")
	if !strings.Contains(line, `"message":"processing task"`) {
		t.Fatalf("expected message field in %s", line)
	}
}

func TestLoggerOutputIsValidJSON(t *testing.T) {
	line := observability.RenderLog("api", "ready")
	if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
		t.Fatalf("expected JSON output, got: %s", line)
	}
}
