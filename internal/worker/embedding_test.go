package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	serviceembedding "github.com/StephenQiu30/hotkey-server/internal/service/embedding"
)

func TestGenerateEmbeddingHandlerCompletesMissingConfigAfterRecordingFailedConfig(t *testing.T) {
	jobQueue := &completeFailQueue{
		job: queue.Job{
			ID:      "job-1",
			Type:    queue.JobTypeGenerateEmbedding,
			Payload: mustJSON(t, queue.GenerateEmbeddingPayload{ItemID: "item-1"}),
		},
		completed: make(chan struct{}),
		failed:    make(chan error, 1),
	}
	handler := NewGenerateEmbeddingHandler(failingEmbeddingService{err: serviceembedding.ErrFailedConfig})
	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeGenerateEmbedding, handler))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	select {
	case <-jobQueue.completed:
	case err := <-jobQueue.failed:
		t.Fatalf("expected terminal completion after failed_config record, got queue failure %v", err)
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("worker did not complete embedding job")
	}
	cancel()
	<-done
}

type failingEmbeddingService struct {
	err error
}

func (s failingEmbeddingService) Generate(context.Context, string) (domainhotspot.Embedding, error) {
	return domainhotspot.Embedding{}, s.err
}
