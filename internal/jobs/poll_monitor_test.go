package jobs

import (
	"context"
	"testing"
	"time"
)

func TestPollMonitorCreatesSuccessfulRun(t *testing.T) {
	runRepo := &fakeRunRepo{}
	connector := &fakeConnector{
		posts: []PostResult{
			{ID: "post-1", Text: "AI agent launch"},
			{ID: "post-2", Text: "OpenAI updates"},
		},
	}
	postRepo := &fakePostRepo{}
	hitRepo := &fakeHitRepo{}

	job := NewPollMonitorJob(runRepo, postRepo, hitRepo, connector)
	err := job.Run(context.Background(), MonitorInfo{
		ID:          42,
		Platform:    "x",
		QueryText:   "openai agent",
		Keywords:    []string{"openai", "agent"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runRepo.lastStatus != "success" {
		t.Fatalf("expected success status, got %q", runRepo.lastStatus)
	}
	if runRepo.lastMonitorID != 42 {
		t.Errorf("expected monitor ID 42, got %d", runRepo.lastMonitorID)
	}
	if runRepo.lastFetchedCount != 2 {
		t.Errorf("expected fetched count 2, got %d", runRepo.lastFetchedCount)
	}
}

func TestPollMonitorRecordsErrorOnFailure(t *testing.T) {
	runRepo := &fakeRunRepo{}
	connector := &fakeConnector{err: errAPIFailure}
	postRepo := &fakePostRepo{}
	hitRepo := &fakeHitRepo{}

	job := NewPollMonitorJob(runRepo, postRepo, hitRepo, connector)
	err := job.Run(context.Background(), MonitorInfo{
		ID:        42,
		Platform:  "x",
		QueryText: "openai agent",
		Keywords:  []string{"openai", "agent"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if runRepo.lastStatus != "failed" {
		t.Fatalf("expected failed status, got %q", runRepo.lastStatus)
	}
}

// fakes for testing

type fakeRunRepo struct {
	lastMonitorID   int64
	lastPlatform    string
	lastStatus      string
	lastFetchedCount int
	lastStoredCount  int
}

func (f *fakeRunRepo) CreateRun(_ context.Context, run MonitorRun) error {
	f.lastMonitorID = run.MonitorID
	f.lastPlatform = run.Platform
	f.lastStatus = run.Status
	f.lastFetchedCount = run.FetchedCount
	f.lastStoredCount = run.StoredCount
	return nil
}

func (f *fakeRunRepo) UpdateRunStatus(_ context.Context, runID int64, status string, errMsg string) error {
	f.lastStatus = status
	return nil
}

type fakeConnector struct {
	posts []PostResult
	err   error
}

func (f *fakeConnector) SearchPosts(_ context.Context, query string, cursor string) ([]PostResult, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	return f.posts, "", nil
}

type fakePostRepo struct {
	stored []PostResult
}

func (f *fakePostRepo) UpsertPost(_ context.Context, post PostResult) error {
	f.stored = append(f.stored, post)
	return nil
}

type fakeHitRepo struct {
	hits []HitResult
}

func (f *fakeHitRepo) UpsertHit(_ context.Context, hit HitResult) error {
	f.hits = append(f.hits, hit)
	return nil
}

var errAPIFailure = &APIError{Code: 500, Message: "internal server error"}

type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return e.Message
}

// Ensure time import is used
var _ = time.Now
