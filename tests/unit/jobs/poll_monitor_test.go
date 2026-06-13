package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/tests/testutil/fake/jobs"
)

func TestPollMonitorCreatesSuccessfulRun(t *testing.T) {
	runRepo := &fakejobs.RunRepo{}
	connector := &fakejobs.Connector{
		Posts: []jobs.PostResult{
			{ID: "post-1", Text: "AI agent launch"},
			{ID: "post-2", Text: "OpenAI updates"},
		},
	}
	postRepo := &fakejobs.PostRepo{}
	hitRepo := &fakejobs.HitRepo{}

	job := jobs.NewPollMonitorJob(runRepo, postRepo, hitRepo, connector, nil)
	err := job.Run(context.Background(), jobs.MonitorInfo{
		ID:        42,
		Platform:  "x",
		QueryText: "openai agent",
		Keywords:  []string{"openai", "agent"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runRepo.UpdateStatus != "success" {
		t.Fatalf("expected success status, got %q", runRepo.UpdateStatus)
	}
	if runRepo.CreateMonitorID != 42 {
		t.Errorf("expected monitor ID 42, got %d", runRepo.CreateMonitorID)
	}
	if runRepo.LastRun.FetchedCount != 2 {
		t.Errorf("expected fetched count 2, got %d", runRepo.LastRun.FetchedCount)
	}
	if runRepo.UpdateCount != 1 {
		t.Errorf("expected 1 update call, got %d", runRepo.UpdateCount)
	}
}

func TestPollMonitorRecordsErrorOnFailure(t *testing.T) {
	runRepo := &fakejobs.RunRepo{}
	connector := &fakejobs.Connector{
		Err: &fakejobs.APIError{Code: 500, Message: "internal server error"},
	}
	postRepo := &fakejobs.PostRepo{}
	hitRepo := &fakejobs.HitRepo{}

	job := jobs.NewPollMonitorJob(runRepo, postRepo, hitRepo, connector, nil)
	err := job.Run(context.Background(), jobs.MonitorInfo{
		ID:        42,
		Platform:  "x",
		QueryText: "openai agent",
		Keywords:  []string{"openai", "agent"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if runRepo.UpdateStatus != "failed" {
		t.Fatalf("expected failed status, got %q", runRepo.UpdateStatus)
	}
	if runRepo.LastRun.ErrorMessage == "" {
		t.Error("expected non-empty error message")
	}
}

func TestPollMonitorCreatesHitsForMatchedPosts(t *testing.T) {
	runRepo := &fakejobs.RunRepo{}
	connector := &fakejobs.Connector{
		Posts: []jobs.PostResult{
			{ID: "post-1", Text: "AI agent launch"},
		},
	}
	postRepo := &fakejobs.PostRepo{}
	hitRepo := &fakejobs.HitRepo{}

	job := jobs.NewPollMonitorJob(runRepo, postRepo, hitRepo, connector, nil)
	err := job.Run(context.Background(), jobs.MonitorInfo{
		ID:        42,
		Platform:  "x",
		QueryText: "openai agent",
		Keywords:  []string{"openai", "agent"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hitRepo.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hitRepo.Hits))
	}
	if hitRepo.Hits[0].MonitorID != 42 {
		t.Errorf("expected monitor ID 42, got %d", hitRepo.Hits[0].MonitorID)
	}
	if hitRepo.Hits[0].PostID != 1 {
		t.Errorf("expected post ID 1, got %d", hitRepo.Hits[0].PostID)
	}
	if len(hitRepo.Hits[0].MatchedKeywords) != 2 {
		t.Errorf("expected 2 matched keywords, got %d", len(hitRepo.Hits[0].MatchedKeywords))
	}
}

func TestPollMonitorScoresHitWhenScorerProvided(t *testing.T) {
	runRepo := &fakejobs.RunRepo{}
	connector := &fakejobs.Connector{
		Posts: []jobs.PostResult{
			{ID: "post-1", Text: "AI agent launch", PublishedAt: time.Now().Add(-30 * time.Minute)},
		},
	}
	postRepo := &fakejobs.PostRepo{}
	hitRepo := &fakejobs.HitRepo{}
	scorer := &fakejobs.Scorer{}

	job := jobs.NewPollMonitorJob(runRepo, postRepo, hitRepo, connector, scorer)
	err := job.Run(context.Background(), jobs.MonitorInfo{
		ID:        10,
		Platform:  "x",
		QueryText: "ai agent",
		Keywords:  []string{"ai", "agent"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scorer.Calls) != 1 {
		t.Fatalf("expected 1 scoring call, got %d", len(scorer.Calls))
	}
	if scorer.Calls[0].PostID != 1 {
		t.Errorf("expected scored post ID 1, got %d", scorer.Calls[0].PostID)
	}
}
