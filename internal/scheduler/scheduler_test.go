package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type recordingProducer struct {
	requests []queue.EnqueueRequest
}

func (p *recordingProducer) Enqueue(_ context.Context, req queue.EnqueueRequest) (queue.Job, error) {
	p.requests = append(p.requests, req)
	return queue.Job{ID: "job-1", Type: req.Type}, nil
}

func TestHourlySchedulerEnqueuesCollectSourceJob(t *testing.T) {
	ctx := context.Background()
	producer := &recordingProducer{}
	now := time.Date(2026, 5, 31, 1, 24, 0, 0, time.UTC)
	s := NewHourlyCollectScheduler(producer, HourlyCollectOptions{
		SourceID: "source-1",
		Now:      func() time.Time { return now },
	})

	if err := s.Tick(ctx); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 1 {
		t.Fatalf("expected one enqueue request, got %d", len(producer.requests))
	}

	req := producer.requests[0]
	if req.Type != queue.JobTypeCollectSource {
		t.Fatalf("expected collect_source job, got %s", req.Type)
	}
	if req.IdempotencyKey != "collect_source:source-1:2026-05-31T01" {
		t.Fatalf("unexpected idempotency key %q", req.IdempotencyKey)
	}

	var payload queue.CollectSourcePayload
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		t.Fatalf("payload was not collect_source payload: %v", err)
	}
	if payload.SourceID != "source-1" {
		t.Fatalf("expected source-1, got %q", payload.SourceID)
	}
	if !payload.ScheduledFor.Equal(time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected scheduled_for to be truncated to the hour, got %s", payload.ScheduledFor)
	}
}
