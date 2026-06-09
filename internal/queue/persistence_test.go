package queue

import (
	"context"
	"testing"
	"time"
)

type fakeJobRepo struct {
	created []Job
	updates []updateCall
}

type updateCall struct {
	id        string
	status    JobStatus
	lastError string
	attempt   int
}

func (r *fakeJobRepo) Create(_ context.Context, job Job) error {
	r.created = append(r.created, job)
	return nil
}

func (r *fakeJobRepo) UpdateStatus(_ context.Context, id string, status JobStatus, lastError string, attempt int) error {
	r.updates = append(r.updates, updateCall{id: id, status: status, lastError: lastError, attempt: attempt})
	return nil
}

func TestJobStatePersisterCreatesOnEnqueue(t *testing.T) {
	repo := &fakeJobRepo{}
	persister := NewJobStatePersister(repo)
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	q := NewMemoryQueue(QueueOptions{
		Now:           func() time.Time { return now },
		MaxAttempts:   3,
		Backoff:       FixedBackoff(time.Minute),
		OnStateChange: persister.OnStateChange,
	})
	payload := mustPayload(t, CollectSourcePayload{SourceID: "source-1", ScheduledFor: now})

	_, err := q.Enqueue(context.Background(), EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect:source-1",
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(repo.created))
	}
	if repo.created[0].Status != JobStatusPending {
		t.Fatalf("expected created status pending, got %s", repo.created[0].Status)
	}
}

func TestJobStatePersisterUpdatesOnClaim(t *testing.T) {
	repo := &fakeJobRepo{}
	persister := NewJobStatePersister(repo)
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	q := NewMemoryQueue(QueueOptions{
		Now:           func() time.Time { return now },
		MaxAttempts:   3,
		Backoff:       FixedBackoff(time.Minute),
		OnStateChange: persister.OnStateChange,
	})
	payload := mustPayload(t, CollectSourcePayload{SourceID: "source-1", ScheduledFor: now})

	_, _ = q.Enqueue(context.Background(), EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect:source-1",
	})
	claimed, err := q.Claim(context.Background())
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(repo.updates))
	}
	if repo.updates[0].status != JobStatusRunning {
		t.Fatalf("expected update status running, got %s", repo.updates[0].status)
	}
	if repo.updates[0].id != claimed.ID {
		t.Fatalf("expected update id %s, got %s", claimed.ID, repo.updates[0].id)
	}
}

func TestJobStatePersisterUpdatesOnRetry(t *testing.T) {
	repo := &fakeJobRepo{}
	persister := NewJobStatePersister(repo)
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	q := NewMemoryQueue(QueueOptions{
		Now:           func() time.Time { return now },
		MaxAttempts:   3,
		Backoff:       FixedBackoff(5 * time.Minute),
		OnStateChange: persister.OnStateChange,
	})
	payload := mustPayload(t, GenerateEmbeddingPayload{ItemID: "item-1"})

	_, _ = q.Enqueue(context.Background(), EnqueueRequest{
		Type:           JobTypeGenerateEmbedding,
		Payload:        payload,
		IdempotencyKey: "embedding:item-1",
	})
	claimed, _ := q.Claim(context.Background())
	_, _ = q.Fail(context.Background(), claimed.ID, NewRetryableError(errTemporary))

	// enqueue(create) + claim(update) + fail(update)
	if len(repo.updates) != 2 {
		t.Fatalf("expected 2 update calls, got %d", len(repo.updates))
	}
	lastUpdate := repo.updates[len(repo.updates)-1]
	if lastUpdate.status != JobStatusScheduled {
		t.Fatalf("expected last update status scheduled, got %s", lastUpdate.status)
	}
	if lastUpdate.attempt != 1 {
		t.Fatalf("expected last update attempt 1, got %d", lastUpdate.attempt)
	}
	if lastUpdate.lastError != "temporary failure" {
		t.Fatalf("expected last error 'temporary failure', got %q", lastUpdate.lastError)
	}
}
