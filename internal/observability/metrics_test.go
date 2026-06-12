package observability

import (
	"testing"
)

func TestCounterIncrements(t *testing.T) {
	c := NewCounter("monitor_runs_total")
	c.Inc()
	if c.Value() != 1 {
		t.Fatalf("expected 1, got %d", c.Value())
	}
}

func TestCounterMultipleIncrements(t *testing.T) {
	c := NewCounter("requests_total")
	c.Inc()
	c.Inc()
	c.Inc()
	if c.Value() != 3 {
		t.Fatalf("expected 3, got %d", c.Value())
	}
}

func TestCounterName(t *testing.T) {
	c := NewCounter("errors_total")
	if c.Name() != "errors_total" {
		t.Fatalf("expected errors_total, got %s", c.Name())
	}
}
