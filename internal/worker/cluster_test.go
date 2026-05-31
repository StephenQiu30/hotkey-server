package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
)

func TestClusterHotspotsHandlerCompletesValidWindow(t *testing.T) {
	start := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	jobQueue := &claimOnceQueue{
		job: queue.Job{
			ID:      "job-1",
			Type:    queue.JobTypeClusterHotspots,
			Payload: mustJSON(t, queue.ClusterHotspotsPayload{WindowStart: start, WindowEnd: end}),
		},
		completed: make(chan struct{}),
	}
	clusterSvc := &recordingClusterService{}
	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeClusterHotspots, NewClusterHotspotsHandler(clusterSvc)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	select {
	case <-jobQueue.completed:
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("worker did not complete cluster job")
	}
	if !clusterSvc.got.Start.Equal(start) || !clusterSvc.got.End.Equal(end) {
		t.Fatalf("unexpected cluster window: %+v", clusterSvc.got)
	}
	cancel()
	<-done
}

type recordingClusterService struct {
	got servicehotspot.Window
}

func (s *recordingClusterService) Cluster(_ context.Context, window servicehotspot.Window) (servicehotspot.Result, error) {
	s.got = window
	return servicehotspot.Result{}, nil
}
