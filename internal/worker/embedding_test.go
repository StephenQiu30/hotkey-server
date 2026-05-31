package worker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	serviceembedding "github.com/StephenQiu30/hotkey-server/internal/service/embedding"
)

func TestGenerateEmbeddingHandlerMarksMissingConfigAsFailedConfig(t *testing.T) {
	jobQueue := &completeFailQueue{
		job: queue.Job{
			ID:      "job-1",
			Type:    queue.JobTypeGenerateEmbedding,
			Payload: mustJSON(t, queue.GenerateEmbeddingPayload{ItemID: "item-1"}),
		},
		failed: make(chan error, 1),
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
	case err := <-jobQueue.failed:
		if !errors.Is(err, serviceembedding.ErrFailedConfig) || !strings.Contains(err.Error(), "failed_config") {
			t.Fatalf("expected failed_config failure, got %v", err)
		}
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("worker did not fail embedding job")
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
