package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/worker"
)

type hourlyRunRepo struct{}

func (h *hourlyRunRepo) TryStart(_ context.Context, _, _ string, _, _ time.Time) (bool, error) {
	return true, nil
}
func (h *hourlyRunRepo) MarkFinished(_ context.Context, _ string, _ time.Time) error { return nil }
func (h *hourlyRunRepo) MarkFailed(_ context.Context, _, _ string, _ time.Time) error { return nil }

func TestHourlyAggregateType(t *testing.T) {
	job := worker.NewHourlyAggregateJob(worker.HourlyAggregateDeps{RunRepo: &hourlyRunRepo{}})
	if job.Type() != "hourly.run" {
		t.Errorf("expected hourly.run, got %s", job.Type())
	}
}

func TestHourlyAggregateDedupeEnabled(t *testing.T) {
	job := worker.NewHourlyAggregateJob(worker.HourlyAggregateDeps{RunRepo: &hourlyRunRepo{}})
	if job.DedupeEnabled() {
		t.Error("expected DedupeEnabled to be false")
	}
}
