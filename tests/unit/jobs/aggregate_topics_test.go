package jobs_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/tests/testutil/fake/jobs"
)

func TestAggregateTopicsCreatesTopicsForSimilarPosts(t *testing.T) {
	provider := &fakejobs.PostCandidateProvider{
		Posts: []jobs.PostCandidate{
			{PostID: 1, Text: "openai agent framework launch"},
			{PostID: 2, Text: "openai agent framework release"},
		},
	}
	persister := &fakejobs.TopicPersister{}
	job := jobs.NewAggregateTopicsJob(provider, persister)

	result, err := job.Run(jobs.AggregateTopicsInput{
		MonitorID: 1,
		RunTime:   time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TopicsCreated != 1 {
		t.Fatalf("expected 1 topic, got %d", result.TopicsCreated)
	}
	if result.PostsClustered != 2 {
		t.Fatalf("expected 2 posts clustered, got %d", result.PostsClustered)
	}
}

func TestAggregateTopicsCreatesSeparateTopicsForDissimilarPosts(t *testing.T) {
	provider := &fakejobs.PostCandidateProvider{
		Posts: []jobs.PostCandidate{
			{PostID: 1, Text: "OpenAI launches new agent framework"},
			{PostID: 2, Text: "Bitcoin price reaches new high"},
		},
	}
	persister := &fakejobs.TopicPersister{}
	job := jobs.NewAggregateTopicsJob(provider, persister)

	result, err := job.Run(jobs.AggregateTopicsInput{
		MonitorID: 1,
		RunTime:   time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TopicsCreated != 2 {
		t.Fatalf("expected 2 topics, got %d", result.TopicsCreated)
	}
}

func TestAggregateTopicsNoPosts(t *testing.T) {
	provider := &fakejobs.PostCandidateProvider{Posts: nil}
	persister := &fakejobs.TopicPersister{}
	job := jobs.NewAggregateTopicsJob(provider, persister)

	result, err := job.Run(jobs.AggregateTopicsInput{
		MonitorID: 1,
		RunTime:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TopicsCreated != 0 {
		t.Fatalf("expected 0 topics, got %d", result.TopicsCreated)
	}
	if result.PostsClustered != 0 {
		t.Fatalf("expected 0 posts clustered, got %d", result.PostsClustered)
	}
}

func TestAggregateTopicsPersistsPostLinks(t *testing.T) {
	provider := &fakejobs.PostCandidateProvider{
		Posts: []jobs.PostCandidate{
			{PostID: 10, Text: "AI agent framework launch"},
			{PostID: 20, Text: "AI agent release update"},
			{PostID: 30, Text: "Crypto market rally"},
		},
	}
	persister := &fakejobs.TopicPersister{}
	job := jobs.NewAggregateTopicsJob(provider, persister)

	result, err := job.Run(jobs.AggregateTopicsInput{
		MonitorID: 5,
		RunTime:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PostsClustered != 3 {
		t.Fatalf("expected 3 posts clustered, got %d", result.PostsClustered)
	}
	if len(persister.PostLinks) != 3 {
		t.Fatalf("expected 3 post links persisted, got %d", len(persister.PostLinks))
	}
}
