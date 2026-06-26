package app

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
)

func TestNewJobRunnerRegistersAuditedScheduledJobs(t *testing.T) {
	runner := newJobRunner(config.Config{}, nil)
	now := time.Date(2026, 6, 26, 8, 9, 0, 0, time.UTC)

	specs := runner.JobSpecs()
	if policy := runner.RetryPolicy(); policy.MaxAttempts != 3 || policy.Backoff != time.Second {
		t.Fatalf("expected production retry policy 3 attempts with 1s backoff, got %+v", policy)
	}
	want := map[string]time.Duration{
		"poll_monitor":           time.Minute,
		"aggregate_topics":       5 * time.Minute,
		"build_snapshots":        10 * time.Minute,
		"dispatch_notifications": time.Minute,
	}
	if len(specs) != len(want) {
		t.Fatalf("expected %d registered jobs, got %d: %+v", len(want), len(specs), specs)
	}

	for _, spec := range specs {
		interval, ok := want[spec.Name]
		if !ok {
			t.Fatalf("unexpected registered job %q", spec.Name)
		}
		if spec.Interval != interval {
			t.Fatalf("expected %s interval %s, got %s", spec.Name, interval, spec.Interval)
		}
		if !spec.HasRunKey {
			t.Fatalf("expected %s to define a run key for idempotent audit", spec.Name)
		}
		runKey := spec.RunKeyAt(now)
		if spec.Name == "poll_monitor" && runKey != "poll_monitor:2026-06-26T08:09" {
			t.Fatalf("expected minute run key for poll_monitor, got %q", runKey)
		}
		if spec.Name == "aggregate_topics" && runKey != "aggregate_topics:2026-06-26T08:09" {
			t.Fatalf("expected minute run key for aggregate_topics, got %q", runKey)
		}
		if spec.Name == "build_snapshots" && runKey != "build_snapshots:2026-06-26T08:09" {
			t.Fatalf("expected minute run key for build_snapshots, got %q", runKey)
		}
		if spec.Name == "dispatch_notifications" && runKey != "dispatch_notifications:2026-06-26T08:09" {
			t.Fatalf("expected minute run key for dispatch_notifications, got %q", runKey)
		}
		delete(want, spec.Name)
	}
	if len(want) > 0 {
		t.Fatalf("missing registered jobs: %+v", want)
	}
}
