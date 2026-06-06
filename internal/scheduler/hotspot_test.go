package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestHotspotSchedulerEnqueuesClusterAndScoreJobs(t *testing.T) {
	ctx := context.Background()
	producer := &recordingProducer{}
	now := time.Date(2026, 6, 7, 10, 30, 0, 0, time.UTC)
	s := NewHotspotScheduler(producer, HotspotSchedulerOptions{
		Now:      func() time.Time { return now },
		Interval: time.Hour,
		Window:   24 * time.Hour,
	})

	if err := s.Tick(ctx); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 2 {
		t.Fatalf("expected two enqueue requests (cluster + score), got %d", len(producer.requests))
	}

	// First should be cluster_hotspots
	clusterReq := producer.requests[0]
	if clusterReq.Type != queue.JobTypeClusterHotspots {
		t.Fatalf("expected cluster_hotspots job, got %s", clusterReq.Type)
	}
	var clusterPayload queue.ClusterHotspotsPayload
	if err := json.Unmarshal(clusterReq.Payload, &clusterPayload); err != nil {
		t.Fatalf("invalid cluster payload: %v", err)
	}
	if clusterPayload.WindowEnd != now.UTC() {
		t.Fatalf("expected window end = now, got %s", clusterPayload.WindowEnd)
	}
	if clusterPayload.WindowStart != now.UTC().Add(-24*time.Hour) {
		t.Fatalf("expected window start = now - 24h, got %s", clusterPayload.WindowStart)
	}
	if clusterReq.IdempotencyKey != "cluster_hotspots:2026-06-07T10" {
		t.Fatalf("unexpected cluster idempotency key: %q", clusterReq.IdempotencyKey)
	}

	// Second should be score_hotspots
	scoreReq := producer.requests[1]
	if scoreReq.Type != queue.JobTypeScoreHotspots {
		t.Fatalf("expected score_hotspots job, got %s", scoreReq.Type)
	}
	var scorePayload queue.ScoreHotspotsPayload
	if err := json.Unmarshal(scoreReq.Payload, &scorePayload); err != nil {
		t.Fatalf("invalid score payload: %v", err)
	}
	if scorePayload.ClusterRunID != clusterReq.IdempotencyKey {
		t.Fatalf("expected score cluster_run_id to match cluster idempotency key, got %q", scorePayload.ClusterRunID)
	}
	if scoreReq.IdempotencyKey != "score_hotspots:2026-06-07T10" {
		t.Fatalf("unexpected score idempotency key: %q", scoreReq.IdempotencyKey)
	}
}

func TestHotspotSchedulerRejectsMissingProducer(t *testing.T) {
	assertPanic(t, func() {
		NewHotspotScheduler(nil, HotspotSchedulerOptions{})
	})
}

func TestHotspotSchedulerDefaultsIntervalAndWindow(t *testing.T) {
	producer := &recordingProducer{}
	s := NewHotspotScheduler(producer, HotspotSchedulerOptions{})
	if s.interval != 30*time.Minute {
		t.Fatalf("expected default interval 30m, got %s", s.interval)
	}
	if s.window != 24*time.Hour {
		t.Fatalf("expected default window 24h, got %s", s.window)
	}
}
