package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestNewRejectsNilQueueAndDefaultsLogger(t *testing.T) {
	assertPanic(t, func() {
		New(nil, nil, nil)
	})

	worker := New(&claimOnceQueue{}, nil, nil)
	if worker.logger == nil {
		t.Fatal("expected default logger")
	}
}

func TestWorkerCompletesClaimedPlaceholderJob(t *testing.T) {
	jobQueue := &claimOnceQueue{
		job:       queue.Job{ID: "job-1", Type: queue.JobTypeCollectSource},
		completed: make(chan struct{}),
	}
	worker := New(jobQueue, nil, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	deadline := time.After(2 * time.Second)
	select {
	case <-jobQueue.completed:
	case err := <-done:
		t.Fatalf("worker exited before completing job: %v", err)
	case <-deadline:
		t.Fatal("worker did not complete claimed job")
	}
	cancel()
	<-done
}

type claimOnceQueue struct {
	job       queue.Job
	claimed   bool
	completed chan struct{}
}

func (q *claimOnceQueue) Claim(context.Context) (queue.Job, error) {
	if q.claimed {
		return queue.Job{}, queue.ErrNoJobs
	}
	q.claimed = true
	return q.job, nil
}

func (q *claimOnceQueue) Complete(_ context.Context, id string) (queue.Job, error) {
	if id != q.job.ID {
		return queue.Job{}, queue.ErrNoJobs
	}
	close(q.completed)
	q.job.Status = queue.JobStatusSucceeded
	return q.job, nil
}

func assertPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
