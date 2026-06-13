package observability_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/observability"
)

func TestCounterIncrements(t *testing.T) {
	c := observability.NewCounter("monitor_runs_total")
	c.Inc()
	if c.Value() != 1 {
		t.Fatalf("expected 1, got %d", c.Value())
	}
}

func TestCounterMultipleIncrements(t *testing.T) {
	c := observability.NewCounter("requests_total")
	c.Inc()
	c.Inc()
	c.Inc()
	if c.Value() != 3 {
		t.Fatalf("expected 3, got %d", c.Value())
	}
}

func TestCounterName(t *testing.T) {
	c := observability.NewCounter("errors_total")
	if c.Name() != "errors_total" {
		t.Fatalf("expected errors_total, got %s", c.Name())
	}
}
