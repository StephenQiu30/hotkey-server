package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrNoJobs = errors.New("no jobs available")

type EnqueueRequest struct {
	Type           JobType
	Payload        json.RawMessage
	IdempotencyKey string
	RunAt          time.Time
}

type BackoffFunc func(attempt int) time.Duration

type QueueOptions struct {
	Now         func() time.Time
	MaxAttempts int
	Backoff     BackoffFunc
}

func FixedBackoff(delay time.Duration) BackoffFunc {
	return func(int) time.Duration {
		return delay
	}
}

type MemoryQueue struct {
	mu          sync.Mutex
	now         func() time.Time
	maxAttempts int
	backoff     BackoffFunc
	nextID      int
	jobs        map[string]Job
	idempotency map[string]string
	order       []string
}

func NewMemoryQueue(opts QueueOptions) *MemoryQueue {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	backoff := opts.Backoff
	if backoff == nil {
		backoff = func(attempt int) time.Duration {
			return time.Duration(attempt) * time.Minute
		}
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &MemoryQueue{
		now:         now,
		maxAttempts: maxAttempts,
		backoff:     backoff,
		jobs:        make(map[string]Job),
		idempotency: make(map[string]string),
	}
}

func (q *MemoryQueue) Enqueue(_ context.Context, req EnqueueRequest) (Job, error) {
	if err := ValidatePayload(req.Type, req.Payload); err != nil {
		return Job{}, err
	}
	if req.IdempotencyKey == "" {
		return Job{}, errors.New("idempotency key is required")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if existingID, ok := q.idempotency[req.IdempotencyKey]; ok {
		return q.jobs[existingID], nil
	}

	q.nextID++
	now := q.now()
	runAt := req.RunAt
	status := JobStatusPending
	if runAt.IsZero() {
		runAt = now
	} else if runAt.After(now) {
		status = JobStatusScheduled
	}
	job := Job{
		ID:             fmt.Sprintf("job-%d", q.nextID),
		Type:           req.Type,
		Payload:        append(json.RawMessage(nil), req.Payload...),
		Status:         status,
		Attempt:        0,
		MaxAttempts:    q.maxAttempts,
		IdempotencyKey: req.IdempotencyKey,
		NextRunAt:      runAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	q.jobs[job.ID] = job
	q.idempotency[job.IdempotencyKey] = job.ID
	q.order = append(q.order, job.ID)
	return job, nil
}

func (q *MemoryQueue) Claim(_ context.Context) (Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := q.now()
	for _, id := range q.order {
		job := q.jobs[id]
		if (job.Status == JobStatusPending || job.Status == JobStatusScheduled) && !job.NextRunAt.After(now) {
			job.Status = JobStatusRunning
			job.UpdatedAt = now
			q.jobs[id] = job
			return job, nil
		}
	}
	return Job{}, ErrNoJobs
}

func (q *MemoryQueue) Fail(_ context.Context, id string, err error) (Job, error) {
	if err == nil {
		return Job{}, errors.New("job failure error is required")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("job %q not found", id)
	}
	now := q.now()
	job.Attempt++
	job.LastError = err.Error()
	job.UpdatedAt = now

	if IsRetryable(err) && job.Attempt < q.maxAttempts {
		job.Status = JobStatusScheduled
		job.NextRunAt = now.Add(q.backoff(job.Attempt))
	} else if IsRetryable(err) {
		job.Status = JobStatusDeadLetter
	} else {
		job.Status = JobStatusFailed
	}
	q.jobs[id] = job
	return job, nil
}

func (q *MemoryQueue) Complete(_ context.Context, id string) (Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("job %q not found", id)
	}
	now := q.now()
	job.Status = JobStatusSucceeded
	job.UpdatedAt = now
	q.jobs[id] = job
	return job, nil
}

func (q *MemoryQueue) PendingLen() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for _, job := range q.jobs {
		if job.Status == JobStatusPending || job.Status == JobStatusScheduled {
			count++
		}
	}
	return count
}

type RetryableError struct {
	err error
}

func NewRetryableError(err error) error {
	return RetryableError{err: err}
}

func (e RetryableError) Error() string {
	return e.err.Error()
}

func (e RetryableError) Unwrap() error {
	return e.err
}

func IsRetryable(err error) bool {
	var retryable RetryableError
	return errors.As(err, &retryable)
}
