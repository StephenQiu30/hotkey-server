package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/service/ingest"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
)

func TestWorkerCollectSourceContinuesWhenSingleItemIngestFails(t *testing.T) {
	sourceSvc := servicesource.NewService(servicesource.NewMemoryRepository())
	createdSource, err := sourceSvc.CreateSource(context.Background(), servicesource.CreateSourceInput{
		Name:             "RSS",
		Type:             servicesource.SourceTypeRSS,
		URL:              "https://example.com/rss.xml",
		FetchIntervalMin: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	jobQueue := &claimOnceQueue{
		job: queue.Job{
			ID:   "job-1",
			Type: queue.JobTypeCollectSource,
			Payload: mustJSON(t, queue.CollectSourcePayload{
				SourceID:     createdSource.ID,
				ScheduledFor: time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC),
			}),
		},
		completed: make(chan struct{}),
	}
	handler := NewCollectSourceHandler(sourceSvc, fakeFetcher{
		items: []fetcher.Item{
			{Title: "有效内容", URL: "https://example.com/a"},
			{Title: "", URL: "https://example.com/invalid"},
			{Title: "另一个有效内容", URL: "https://example.com/b"},
		},
	}, &countingIngest{})
	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeCollectSource, handler))

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
		t.Fatal("worker did not complete collect job")
	}

	runs, err := sourceSvc.ListCollectionRuns(context.Background(), createdSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != servicesource.CollectionRunStatusSuccess || runs[0].ItemsFetched != 2 {
		t.Fatalf("expected successful run with two ingested items, got %+v", runs)
	}
	cancel()
	<-done
}

type fakeFetcher struct {
	items []fetcher.Item
	err   error
}

func (f fakeFetcher) Fetch(context.Context, fetcher.Source) ([]fetcher.Item, error) {
	return f.items, f.err
}

type countingIngest struct {
	count int
}

func (i *countingIngest) Ingest(_ context.Context, input CollectIngestInput) (ingest.Result, error) {
	if input.Title == "" {
		return ingest.Result{}, errors.New("invalid title")
	}
	i.count++
	return ingest.Result{}, nil
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
