package worker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
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

type dummyHandler struct{}

func (h *dummyHandler) Handle(context.Context, queue.Job) error { return nil }

func TestWorkerCompletesClaimedPlaceholderJob(t *testing.T) {
	jobQueue := &claimOnceQueue{
		job:       queue.Job{ID: "job-1", Type: queue.JobTypeCollectSource},
		completed: make(chan struct{}),
	}
	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeCollectSource, &dummyHandler{}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	deadline := time.After(100 * time.Millisecond)
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

func TestWorkerMarksClaimedJobFailedWhenCompleteFails(t *testing.T) {
	expectedErr := errors.New("complete failed")
	jobQueue := &completeFailQueue{
		job:         queue.Job{ID: "job-1", Type: queue.JobTypeCollectSource},
		completeErr: expectedErr,
		failed:      make(chan error, 1),
	}
	worker := New(jobQueue, nil, slog.Default(), WithHandler(queue.JobTypeCollectSource, &dummyHandler{}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	select {
	case err := <-jobQueue.failed:
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected fallback failure reason %v, got %v", expectedErr, err)
		}
	case err := <-done:
		t.Fatalf("worker exited before marking job failed: %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("worker did not mark claimed job failed")
	}
	cancel()
	<-done
}

func TestWorkerMarksClaimedJobFailedWhenNoHandlerRegistered(t *testing.T) {
	jobQueue := &completeFailQueue{
		job:    queue.Job{ID: "job-1", Type: "unknown_type"},
		failed: make(chan error, 1),
	}
	worker := New(jobQueue, nil, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	select {
	case err := <-jobQueue.failed:
		if err == nil || !strings.Contains(err.Error(), "no handler registered") {
			t.Fatalf("expected no handler error, got %v", err)
		}
	case err := <-done:
		t.Fatalf("worker exited early: %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("worker did not mark unhandled job failed")
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

func (q *claimOnceQueue) Fail(context.Context, string, error) (queue.Job, error) {
	return queue.Job{}, nil
}

type completeFailQueue struct {
	job         queue.Job
	claimed     bool
	completeErr error
	completed   chan struct{}
	failed      chan error
}

func (q *completeFailQueue) Claim(context.Context) (queue.Job, error) {
	if q.claimed {
		return queue.Job{}, queue.ErrNoJobs
	}
	q.claimed = true
	return q.job, nil
}

func (q *completeFailQueue) Complete(context.Context, string) (queue.Job, error) {
	if q.completeErr == nil && q.completed != nil {
		close(q.completed)
	}
	return queue.Job{}, q.completeErr
}

func (q *completeFailQueue) Fail(_ context.Context, id string, err error) (queue.Job, error) {
	if id != q.job.ID {
		return queue.Job{}, queue.ErrNoJobs
	}
	q.failed <- err
	q.job.Status = queue.JobStatusFailed
	return q.job, nil
}

func TestWorkerLogsRedisConnectionErrorOnClaim(t *testing.T) {
	redisErr := queue.NewRedisConnectionError(errors.New("dial tcp 127.0.0.1:6379: connect: connection refused"))
	jobQueue := &claimFailQueue{claimErr: redisErr}
	worker := New(jobQueue, nil, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	// Worker should log the error but not exit
	select {
	case <-time.After(150 * time.Millisecond):
		// Expected: worker continues running, just logs the error
	case err := <-done:
		t.Fatalf("worker should not exit on Redis connection error, got: %v", err)
	}
	cancel()
	<-done
}

type claimFailQueue struct {
	claimErr error
}

func (q *claimFailQueue) Claim(context.Context) (queue.Job, error) {
	return queue.Job{}, q.claimErr
}

func (q *claimFailQueue) Complete(context.Context, string) (queue.Job, error) {
	return queue.Job{}, nil
}

func (q *claimFailQueue) Fail(context.Context, string, error) (queue.Job, error) {
	return queue.Job{}, nil
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
