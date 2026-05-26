package realtime

import (
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/event"
)

func TestAuthorizedRealtimePushEntersEventCandidateWithLatency(t *testing.T) {
	now := time.Date(2026, 5, 26, 10, 30, 0, 0, time.UTC)
	eventService := event.NewService(event.Options{VectorEnabled: true})
	service := NewService(eventService, Options{
		Now:                func() time.Time { return now.Add(150 * time.Millisecond) },
		RateLimitWindow:    time.Minute,
		MaxEventsPerWindow: 10,
		FailureThreshold:   3,
	})

	if err := service.RegisterSource(Source{
		ID:         "openai-realtime",
		Token:      "valid-token",
		Enabled:    true,
		LowLatency: true,
	}); err != nil {
		t.Fatalf("register source: %v", err)
	}

	result, err := service.AcceptPush(PushInput{
		SourceID:     "openai-realtime",
		Token:        "valid-token",
		SourceItemID: "rt_1",
		Title:        "OpenAI releases realtime model",
		ContentHash:  "hash-rt-1",
		ReceivedAt:   now,
		Vector:       []float64{0.91, 0.12},
	})
	if err != nil {
		t.Fatalf("AcceptPush returned error: %v", err)
	}
	if result.Status != StatusAccepted {
		t.Fatalf("status = %s, want %s", result.Status, StatusAccepted)
	}
	if result.Match.ClusterID == "" {
		t.Fatalf("cluster id is empty: %#v", result.Match)
	}
	if result.LatencyMilliseconds > 1000 {
		t.Fatalf("latency = %dms, want <= 1000ms", result.LatencyMilliseconds)
	}
	if len(eventService.ListClusters()) != 1 {
		t.Fatalf("event candidates were not clustered")
	}
}

func TestRealtimePushRateLimitFallsBackToQueue(t *testing.T) {
	now := time.Date(2026, 5, 26, 10, 30, 0, 0, time.UTC)
	service := NewService(event.NewService(event.Options{}), Options{
		Now:                func() time.Time { return now },
		RateLimitWindow:    time.Minute,
		MaxEventsPerWindow: 1,
		FailureThreshold:   3,
	})
	if err := service.RegisterSource(Source{ID: "source", Token: "token", Enabled: true}); err != nil {
		t.Fatalf("register source: %v", err)
	}

	_, err := service.AcceptPush(PushInput{SourceID: "source", Token: "token", SourceItemID: "rt_1", Title: "Realtime AI event", ContentHash: "hash-1", ReceivedAt: now})
	if err != nil {
		t.Fatalf("first push: %v", err)
	}
	result, err := service.AcceptPush(PushInput{SourceID: "source", Token: "token", SourceItemID: "rt_2", Title: "Realtime AI event 2", ContentHash: "hash-2", ReceivedAt: now})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want %v", err, ErrRateLimited)
	}
	if result.Status != StatusDegraded || result.FallbackReason != FallbackRateLimited {
		t.Fatalf("fallback result = %#v", result)
	}
	if queued := service.ListFallbacks(); len(queued) != 1 || queued[0].SourceItemID != "rt_2" {
		t.Fatalf("fallback queue = %#v", queued)
	}
}

func TestRealtimeCircuitBreakerFallsBackAfterFailures(t *testing.T) {
	now := time.Date(2026, 5, 26, 10, 30, 0, 0, time.UTC)
	service := NewService(event.NewService(event.Options{}), Options{
		Now:                func() time.Time { return now },
		RateLimitWindow:    time.Minute,
		MaxEventsPerWindow: 10,
		FailureThreshold:   2,
	})
	if err := service.RegisterSource(Source{ID: "source", Token: "token", Enabled: true}); err != nil {
		t.Fatalf("register source: %v", err)
	}

	service.RecordFailure("source")
	service.RecordFailure("source")

	result, err := service.AcceptPush(PushInput{SourceID: "source", Token: "token", SourceItemID: "rt_1", Title: "Realtime AI event", ContentHash: "hash-1", ReceivedAt: now})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want %v", err, ErrCircuitOpen)
	}
	if result.Status != StatusDegraded || result.FallbackReason != FallbackCircuitOpen {
		t.Fatalf("fallback result = %#v", result)
	}
	if source := service.GetSource("source"); source.CircuitStatus != CircuitOpen {
		t.Fatalf("circuit = %s, want %s", source.CircuitStatus, CircuitOpen)
	}
}

func TestRealtimeRejectsUnauthorizedSource(t *testing.T) {
	service := NewService(event.NewService(event.Options{}), Options{})
	if err := service.RegisterSource(Source{ID: "source", Token: "token", Enabled: true}); err != nil {
		t.Fatalf("register source: %v", err)
	}

	_, err := service.AcceptPush(PushInput{SourceID: "source", Token: "wrong", SourceItemID: "rt_1", Title: "Realtime AI event", ContentHash: "hash-1"})
	if !errors.Is(err, ErrUnauthorizedSource) {
		t.Fatalf("err = %v, want %v", err, ErrUnauthorizedSource)
	}
}
