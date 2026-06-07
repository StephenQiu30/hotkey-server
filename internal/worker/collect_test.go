package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
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

func TestCollectSourceHandlerUsesAdapterAndPassesMetadataOnly(t *testing.T) {
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
			ID:   "job-2",
			Type: queue.JobTypeCollectSource,
			Payload: mustJSON(t, queue.CollectSourcePayload{
				SourceID:     createdSource.ID,
				ScheduledFor: time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC),
			}),
		},
		completed: make(chan struct{}),
	}

	collector := &stubAdapterCollector{
		items: []adapter.NormalizedItem{
			{Title: "Free Article", URL: "https://example.com/free", Snippet: "snippet", Language: "en", IdempotencyKey: "key-1"},
			{Title: "Paywall Article", URL: "https://example.com/paywall", Snippet: "[metadata_only] snippet", Language: "zh", MetadataOnly: true, IdempotencyKey: "key-2"},
		},
	}
	ingester := &recordingIngest{}
	handler := NewCollectSourceHandlerWithAdapter(sourceSvc, collector, ingester)
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

	if len(ingester.inputs) != 2 {
		t.Fatalf("expected 2 ingest calls, got %d", len(ingester.inputs))
	}

	// First item: free content
	if ingester.inputs[0].MetadataOnly {
		t.Fatal("expected first item MetadataOnly=false")
	}
	if ingester.inputs[0].Language != "en" {
		t.Fatalf("expected first item language %q, got %q", "en", ingester.inputs[0].Language)
	}

	// Second item: paywall content
	if !ingester.inputs[1].MetadataOnly {
		t.Fatal("expected second item MetadataOnly=true")
	}
	if ingester.inputs[1].Language != "zh" {
		t.Fatalf("expected second item language %q, got %q", "zh", ingester.inputs[1].Language)
	}

	runs, err := sourceSvc.ListCollectionRuns(context.Background(), createdSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != servicesource.CollectionRunStatusSuccess || runs[0].ItemsFetched != 2 {
		t.Fatalf("expected successful run with 2 items, got %+v", runs)
	}

	cancel()
	<-done
}

type stubAdapterCollector struct {
	items []adapter.NormalizedItem
	err   error
}

func (c *stubAdapterCollector) Collect(_ context.Context, _ adapter.CollectInput) (adapter.CollectOutput, error) {
	return adapter.CollectOutput{Items: c.items}, c.err
}

type recordingIngest struct {
	inputs []CollectIngestInput
}

func (r *recordingIngest) Ingest(_ context.Context, input CollectIngestInput) (ingest.Result, error) {
	r.inputs = append(r.inputs, input)
	return ingest.Result{Created: true}, nil
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
