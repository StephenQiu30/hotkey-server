package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
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

func TestHourlySchedulerRejectsMissingRequiredInputs(t *testing.T) {
	assertPanic(t, func() {
		NewHourlyCollectScheduler(nil, HourlyCollectOptions{SourceID: "source-1"})
	})
	assertPanic(t, func() {
		NewHourlyCollectScheduler(&recordingProducer{}, HourlyCollectOptions{})
	})
}

func TestHourlySchedulerContinuesAfterTransientTickError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	producer := &flakyProducer{failuresLeft: 1}
	now := time.Date(2026, 5, 31, 1, 24, 0, 0, time.UTC)
	s := NewHourlyCollectScheduler(producer, HourlyCollectOptions{
		SourceID: "source-1",
		Now:      func() time.Time { return now },
		Interval: time.Millisecond,
	})

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	deadline := time.After(time.Second)
	for producer.successes() == 0 {
		select {
		case err := <-done:
			t.Fatalf("scheduler exited on transient error: %v", err)
		case <-deadline:
			t.Fatal("scheduler did not continue after transient error")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	<-done
}

func assertPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

type flakyProducer struct {
	mu           sync.Mutex
	failuresLeft int
	successCount int
}

func (p *flakyProducer) Enqueue(_ context.Context, req queue.EnqueueRequest) (queue.Job, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failuresLeft > 0 {
		p.failuresLeft--
		return queue.Job{}, errors.New("temporary enqueue failure")
	}
	p.successCount++
	return queue.Job{ID: "job-1", Type: req.Type}, nil
}

func (p *flakyProducer) successes() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.successCount
}
