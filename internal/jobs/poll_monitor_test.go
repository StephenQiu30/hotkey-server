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
		ID:        42,
		Platform:  "x",
		QueryText: "openai agent",
		Keywords:  []string{"openai", "agent"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runRepo.updateStatus != "success" {
		t.Fatalf("expected success status, got %q", runRepo.updateStatus)
	}
	if runRepo.createMonitorID != 42 {
		t.Errorf("expected monitor ID 42, got %d", runRepo.createMonitorID)
	}
	if runRepo.updateRun.FetchedCount != 2 {
		t.Errorf("expected fetched count 2, got %d", runRepo.updateRun.FetchedCount)
	}
	if runRepo.updateCount != 1 {
		t.Errorf("expected 1 update call, got %d", runRepo.updateCount)
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
	if runRepo.updateStatus != "failed" {
		t.Fatalf("expected failed status, got %q", runRepo.updateStatus)
	}
	if runRepo.updateRun.ErrorMessage == "" {
		t.Error("expected non-empty error message")
	}
}

func TestPollMonitorCreatesHitsForMatchedPosts(t *testing.T) {
	runRepo := &fakeRunRepo{}
	connector := &fakeConnector{
		posts: []PostResult{
			{ID: "post-1", Text: "AI agent launch"},
		},
	}
	postRepo := &fakePostRepo{nextID: 100}
	hitRepo := &fakeHitRepo{}

	job := NewPollMonitorJob(runRepo, postRepo, hitRepo, connector)
	err := job.Run(context.Background(), MonitorInfo{
		ID:        42,
		Platform:  "x",
		QueryText: "openai agent",
		Keywords:  []string{"openai", "agent"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hitRepo.hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hitRepo.hits))
	}
	if hitRepo.hits[0].MonitorID != 42 {
		t.Errorf("expected monitor ID 42, got %d", hitRepo.hits[0].MonitorID)
	}
	if hitRepo.hits[0].PostID != 101 {
		t.Errorf("expected post ID 101, got %d", hitRepo.hits[0].PostID)
	}
	if len(hitRepo.hits[0].MatchedKeywords) != 2 {
		t.Errorf("expected 2 matched keywords, got %d", len(hitRepo.hits[0].MatchedKeywords))
	}
}

// fakes for testing

type fakeRunRepo struct {
	createCount     int
	createMonitorID int64
	updateCount     int
	updateRunID     int64
	updateStatus    string
	updateRun       MonitorRun
}

func (f *fakeRunRepo) CreateRun(_ context.Context, run MonitorRun) (int64, error) {
	f.createCount++
	f.createMonitorID = run.MonitorID
	return int64(f.createCount), nil
}

func (f *fakeRunRepo) UpdateRun(_ context.Context, runID int64, run MonitorRun) error {
	f.updateCount++
	f.updateRunID = runID
	f.updateStatus = run.Status
	f.updateRun = run
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
	nextID int64
}

func (f *fakePostRepo) UpsertPost(_ context.Context, post PostResult) (int64, error) {
	f.stored = append(f.stored, post)
	f.nextID++
	return f.nextID, nil
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
