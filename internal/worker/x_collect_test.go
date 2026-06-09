package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
)

// --- Test: X source successful collection ---

func TestCollectSourceHandlerXSuccessRecordsRunAndIngestsItems(t *testing.T) {
	sourceSvc := servicesource.NewService(servicesource.NewMemoryRepository())
	createdSource, err := sourceSvc.CreateSource(context.Background(), servicesource.CreateSourceInput{
		Name:             "X-AI",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote:   "X public API v2.",
		FetchIntervalMin: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	jobQueue := &claimOnceQueue{
		job: queue.Job{
			ID:   "job-x-1",
			Type: queue.JobTypeCollectSource,
			Payload: mustJSON(t, queue.CollectSourcePayload{
				SourceID:     createdSource.ID,
				ScheduledFor: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
			}),
		},
		completed: make(chan struct{}),
	}

	ingester := &recordingIngest{}
	handler := NewCollectSourceHandler(sourceSvc, &noopFetcher{}, ingester)
	handler.SetCredentialResolver(&stubCredentialResolver{token: "valid_token"})
	handler.xFetcherFn = func(_ string) SourceFetcher {
		return &xStubFetcher{
			items: []fetcher.Item{
				{Title: "GPT-5 发布", URL: "https://x.com/i/status/111", ExternalID: "111"},
				{Title: "Gemini 2.5 更新", URL: "https://x.com/i/status/222", ExternalID: "222"},
			},
		}
	}

	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeCollectSource, handler))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	select {
	case <-jobQueue.completed:
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("worker did not complete collect job")
	}

	if len(ingester.inputs) != 2 {
		t.Fatalf("expected 2 ingest calls, got %d", len(ingester.inputs))
	}
	if ingester.inputs[0].Title != "GPT-5 发布" {
		t.Fatalf("expected first item title %q, got %q", "GPT-5 发布", ingester.inputs[0].Title)
	}

	runs, err := sourceSvc.ListCollectionRuns(context.Background(), createdSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 collection run, got %d", len(runs))
	}
	if runs[0].Status != servicesource.CollectionRunStatusSuccess {
		t.Fatalf("expected success status, got %q", runs[0].Status)
	}
	if runs[0].ItemsFetched != 2 {
		t.Fatalf("expected 2 items fetched, got %d", runs[0].ItemsFetched)
	}
	if runs[0].ErrorType != servicesource.CollectionRunErrorTypeNone {
		t.Fatalf("expected no error type for success, got %q", runs[0].ErrorType)
	}
	cancel()
	<-done
}

// --- Test: X source auth failure (no credential) ---

func TestCollectSourceHandlerXAuthFailureNoCredential(t *testing.T) {
	sourceSvc := servicesource.NewService(servicesource.NewMemoryRepository())
	createdSource, err := sourceSvc.CreateSource(context.Background(), servicesource.CreateSourceInput{
		Name:             "X-AI",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote:   "X public API v2.",
		FetchIntervalMin: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	jobQueue := &claimOnceSignalQueue{
		job: queue.Job{
			ID:   "job-x-auth",
			Type: queue.JobTypeCollectSource,
			Payload: mustJSON(t, queue.CollectSourcePayload{
				SourceID:     createdSource.ID,
				ScheduledFor: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
			}),
		},
		done: make(chan struct{}),
	}

	handler := NewCollectSourceHandler(sourceSvc, &noopFetcher{}, &countingIngest{})
	handler.SetCredentialResolver(&stubCredentialResolver{err: servicesource.ErrNotFound})

	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeCollectSource, handler))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	select {
	case <-jobQueue.done:
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("worker did not complete collect job")
	}

	runs, err := sourceSvc.ListCollectionRuns(context.Background(), createdSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 collection run, got %d", len(runs))
	}
	if runs[0].Status != servicesource.CollectionRunStatusFailed {
		t.Fatalf("expected failed status, got %q", runs[0].Status)
	}
	if runs[0].ErrorType != servicesource.CollectionRunErrorTypeAuthFailed {
		t.Fatalf("expected auth_failed error type, got %q", runs[0].ErrorType)
	}
	cancel()
	<-done
}

// --- Test: X source rate limit failure ---

func TestCollectSourceHandlerXRateLimitFailure(t *testing.T) {
	sourceSvc := servicesource.NewService(servicesource.NewMemoryRepository())
	createdSource, err := sourceSvc.CreateSource(context.Background(), servicesource.CreateSourceInput{
		Name:             "X-AI",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote:   "X public API v2.",
		FetchIntervalMin: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	jobQueue := &claimOnceSignalQueue{
		job: queue.Job{
			ID:   "job-x-rl",
			Type: queue.JobTypeCollectSource,
			Payload: mustJSON(t, queue.CollectSourcePayload{
				SourceID:     createdSource.ID,
				ScheduledFor: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
			}),
		},
		done: make(chan struct{}),
	}

	handler := NewCollectSourceHandler(sourceSvc, &noopFetcher{}, &countingIngest{})
	handler.SetCredentialResolver(&stubCredentialResolver{token: "valid_token"})
	handler.xFetcherFn = func(_ string) SourceFetcher {
		return &xStubFetcher{
			err: &fetcher.RateLimitError{ResetAt: time.Date(2026, 6, 9, 1, 0, 0, 0, time.UTC)},
		}
	}

	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeCollectSource, handler))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	select {
	case <-jobQueue.done:
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("worker did not complete collect job")
	}

	runs, err := sourceSvc.ListCollectionRuns(context.Background(), createdSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 collection run, got %d", len(runs))
	}
	if runs[0].Status != servicesource.CollectionRunStatusFailed {
		t.Fatalf("expected failed status, got %q", runs[0].Status)
	}
	if runs[0].ErrorType != servicesource.CollectionRunErrorTypeRateLimited {
		t.Fatalf("expected rate_limited error type, got %q", runs[0].ErrorType)
	}
	cancel()
	<-done
}

// --- Test: X source generic fetch error ---

func TestCollectSourceHandlerXGenericFetchError(t *testing.T) {
	sourceSvc := servicesource.NewService(servicesource.NewMemoryRepository())
	createdSource, err := sourceSvc.CreateSource(context.Background(), servicesource.CreateSourceInput{
		Name:             "X-AI",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote:   "X public API v2.",
		FetchIntervalMin: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	jobQueue := &claimOnceSignalQueue{
		job: queue.Job{
			ID:   "job-x-err",
			Type: queue.JobTypeCollectSource,
			Payload: mustJSON(t, queue.CollectSourcePayload{
				SourceID:     createdSource.ID,
				ScheduledFor: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
			}),
		},
		done: make(chan struct{}),
	}

	handler := NewCollectSourceHandler(sourceSvc, &noopFetcher{}, &countingIngest{})
	handler.SetCredentialResolver(&stubCredentialResolver{token: "valid_token"})
	handler.xFetcherFn = func(_ string) SourceFetcher {
		return &xStubFetcher{
			err: errors.New("network timeout"),
		}
	}

	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeCollectSource, handler))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	select {
	case <-jobQueue.done:
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("worker did not complete collect job")
	}

	runs, err := sourceSvc.ListCollectionRuns(context.Background(), createdSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 collection run, got %d", len(runs))
	}
	if runs[0].Status != servicesource.CollectionRunStatusFailed {
		t.Fatalf("expected failed status, got %q", runs[0].Status)
	}
	if runs[0].ErrorType != servicesource.CollectionRunErrorTypeGeneric {
		t.Fatalf("expected generic error type, got %q", runs[0].ErrorType)
	}
	cancel()
	<-done
}

// --- Test: X source auth error from API (401/403) ---

func TestCollectSourceHandlerXAuthErrorFromAPI(t *testing.T) {
	sourceSvc := servicesource.NewService(servicesource.NewMemoryRepository())
	createdSource, err := sourceSvc.CreateSource(context.Background(), servicesource.CreateSourceInput{
		Name:             "X-AI",
		Type:             servicesource.SourceTypeX,
		URL:              "https://api.x.com/2/tweets/search/recent?query=AI",
		ComplianceNote:   "X public API v2.",
		FetchIntervalMin: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	jobQueue := &claimOnceSignalQueue{
		job: queue.Job{
			ID:   "job-x-api-auth",
			Type: queue.JobTypeCollectSource,
			Payload: mustJSON(t, queue.CollectSourcePayload{
				SourceID:     createdSource.ID,
				ScheduledFor: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
			}),
		},
		done: make(chan struct{}),
	}

	handler := NewCollectSourceHandler(sourceSvc, &noopFetcher{}, &countingIngest{})
	handler.SetCredentialResolver(&stubCredentialResolver{token: "expired_token"})
	handler.xFetcherFn = func(_ string) SourceFetcher {
		return &xStubFetcher{
			err: &fetcher.AuthError{Message: "invalid or expired access token"},
		}
	}

	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeCollectSource, handler))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	select {
	case <-jobQueue.done:
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("worker did not complete collect job")
	}

	runs, err := sourceSvc.ListCollectionRuns(context.Background(), createdSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 collection run, got %d", len(runs))
	}
	if runs[0].Status != servicesource.CollectionRunStatusFailed {
		t.Fatalf("expected failed status, got %q", runs[0].Status)
	}
	if runs[0].ErrorType != servicesource.CollectionRunErrorTypeAuthFailed {
		t.Fatalf("expected auth_failed error type, got %q", runs[0].ErrorType)
	}
	cancel()
	<-done
}

// --- Test helpers ---

// noopFetcher is a fetcher that returns nothing (used as placeholder for non-X paths).
type noopFetcher struct{}

func (f *noopFetcher) Fetch(context.Context, fetcher.Source) ([]fetcher.Item, error) {
	return nil, nil
}

// xStubFetcher returns configured items or error (used to simulate X API responses).
type xStubFetcher struct {
	items []fetcher.Item
	err   error
}

func (f *xStubFetcher) Fetch(_ context.Context, source fetcher.Source) ([]fetcher.Item, error) {
	return f.items, f.err
}

// stubCredentialResolver returns a preconfigured token or error.
type stubCredentialResolver struct {
	token string
	err   error
}

func (r *stubCredentialResolver) ResolveAccessToken(_ context.Context, _ string) (string, error) {
	return r.token, r.err
}

// claimOnceSignalQueue signals done on both Complete and Fail (for tests that expect handler failures).
type claimOnceSignalQueue struct {
	job     queue.Job
	claimed bool
	done    chan struct{}
	once    sync.Once
}

func (q *claimOnceSignalQueue) signal() {
	q.once.Do(func() { close(q.done) })
}

func (q *claimOnceSignalQueue) Claim(context.Context) (queue.Job, error) {
	if q.claimed {
		return queue.Job{}, queue.ErrNoJobs
	}
	q.claimed = true
	return q.job, nil
}

func (q *claimOnceSignalQueue) Complete(_ context.Context, _ string) (queue.Job, error) {
	q.signal()
	q.job.Status = queue.JobStatusSucceeded
	return q.job, nil
}

func (q *claimOnceSignalQueue) Fail(_ context.Context, _ string, _ error) (queue.Job, error) {
	q.signal()
	q.job.Status = queue.JobStatusFailed
	return q.job, nil
}
