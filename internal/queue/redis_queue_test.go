package queue

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

var errTemporary = errors.New("temporary failure")

type fakeRedisStore struct {
	mu     sync.Mutex
	values map[string][]byte
	lists  map[string][][]byte
}

func newFakeRedisStore() *fakeRedisStore {
	return &fakeRedisStore{
		values: make(map[string][]byte),
		lists:  make(map[string][][]byte),
	}
}

func (s *fakeRedisStore) SetNX(_ context.Context, key string, value []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[key]; ok {
		return false, nil
	}
	s.values[key] = append([]byte(nil), value...)
	return true, nil
}

func (s *fakeRedisStore) Set(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *fakeRedisStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.values[key]...), nil
}

func (s *fakeRedisStore) Del(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	delete(s.lists, key)
	return nil
}

func (s *fakeRedisStore) LPush(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists[key] = append([][]byte{append([]byte(nil), value...)}, s.lists[key]...)
	return nil
}

type failingRedisStore struct {
	err error
}

func (s *failingRedisStore) Set(context.Context, string, []byte) error           { return s.err }
func (s *failingRedisStore) SetNX(context.Context, string, []byte) (bool, error) { return false, s.err }
func (s *failingRedisStore) Del(context.Context, string) error                   { return s.err }
func (s *failingRedisStore) Get(context.Context, string) ([]byte, error)         { return nil, s.err }
func (s *failingRedisStore) LPush(context.Context, string, []byte) error         { return s.err }
func (s *failingRedisStore) RPop(context.Context, string) ([]byte, error)        { return nil, s.err }

var errRedisNil = errors.New("redis nil reply")

func (s *fakeRedisStore) RPop(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.lists[key]
	if len(list) == 0 {
		return nil, errRedisNil
	}
	value := list[len(list)-1]
	s.lists[key] = list[:len(list)-1]
	return append([]byte(nil), value...), nil
}

func TestRedisQueueEnqueueIdempotentlyAndClaim(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	store := newFakeRedisStore()
	q := NewRedisQueue(store, RedisQueueOptions{
		QueueName: "hotkey:test",
		Now:       func() time.Time { return now },
	})
	payload := mustPayload(t, CollectSourcePayload{SourceID: "source-1", ScheduledFor: now})

	first, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect_source:source-1:2026-05-31T01",
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	second, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect_source:source-1:2026-05-31T01",
	})
	if err != nil {
		t.Fatalf("duplicate enqueue failed: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected duplicate to return existing job %q, got %q", first.ID, second.ID)
	}

	claimed, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claimed.ID != first.ID || claimed.Status != JobStatusRunning {
		body, _ := json.Marshal(claimed)
		t.Fatalf("unexpected claimed job %s", body)
	}
	if _, err := q.Claim(ctx); err != ErrNoJobs {
		t.Fatalf("expected duplicate enqueue to leave one queue item, got %v", err)
	}
}

func TestRedisQueueEnqueueReturnsRedisConnectionErrorWhenStoreDown(t *testing.T) {
	ctx := context.Background()
	store := &failingRedisStore{err: errors.New("dial tcp 127.0.0.1:6379: connect: connection refused")}
	q := NewRedisQueue(store, RedisQueueOptions{QueueName: "hotkey:test"})
	payload := mustPayload(t, CollectSourcePayload{SourceID: "source-1", ScheduledFor: time.Now()})

	_, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect:source-1",
	})
	if err == nil {
		t.Fatal("expected error when Redis is down")
	}
	if !IsRedisConnectionError(err) {
		t.Fatalf("expected RedisConnectionError, got %T: %v", err, err)
	}
}

func TestRedisQueueClaimReturnsRedisConnectionErrorWhenStoreDown(t *testing.T) {
	ctx := context.Background()
	store := &failingRedisStore{err: errors.New("dial tcp 127.0.0.1:6379: connect: connection refused")}
	q := NewRedisQueue(store, RedisQueueOptions{QueueName: "hotkey:test"})

	_, err := q.Claim(ctx)
	if err == nil {
		t.Fatal("expected error when Redis is down")
	}
	if !IsRedisConnectionError(err) {
		t.Fatalf("expected RedisConnectionError, got %T: %v", err, err)
	}
}

func TestRedisQueueFailReturnsRedisConnectionErrorWhenStoreDown(t *testing.T) {
	ctx := context.Background()
	store := &failingRedisStore{err: errors.New("dial tcp 127.0.0.1:6379: connect: connection refused")}
	q := NewRedisQueue(store, RedisQueueOptions{QueueName: "hotkey:test"})

	_, err := q.Fail(ctx, "job-1", errors.New("handler error"))
	if err == nil {
		t.Fatal("expected error when Redis is down")
	}
	if !IsRedisConnectionError(err) {
		t.Fatalf("expected RedisConnectionError, got %T: %v", err, err)
	}
}

func TestRedisQueuePendingLenReturnsZero(t *testing.T) {
	store := newFakeRedisStore()
	q := NewRedisQueue(store, RedisQueueOptions{QueueName: "hotkey:test"})

	// RedisQueue 不维护内存计数，PendingLen 始终返回 0。
	// 生产环境通过 Redis CLI 或监控获取精确值。
	if got := q.PendingLen(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestRedisQueuePersistenceCallbackFiresOnStateChange(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	store := newFakeRedisStore()
	var persisted []Job
	q := NewRedisQueue(store, RedisQueueOptions{
		QueueName: "hotkey:test",
		Now:       func() time.Time { return now },
		OnStateChange: func(_ context.Context, job Job) {
			persisted = append(persisted, job)
		},
	})
	payload := mustPayload(t, CollectSourcePayload{SourceID: "source-1", ScheduledFor: now})

	job, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect:source-1",
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persistence call after enqueue, got %d", len(persisted))
	}
	if persisted[0].Status != JobStatusPending {
		t.Fatalf("expected persisted status pending, got %s", persisted[0].Status)
	}

	claimed, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("expected 2 persistence calls after claim, got %d", len(persisted))
	}
	if persisted[1].Status != JobStatusRunning {
		t.Fatalf("expected persisted status running, got %s", persisted[1].Status)
	}

	completed, err := q.Complete(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(persisted) != 3 {
		t.Fatalf("expected 3 persistence calls after complete, got %d", len(persisted))
	}
	if persisted[2].Status != JobStatusSucceeded {
		t.Fatalf("expected persisted status succeeded, got %s", persisted[2].Status)
	}
	_ = job
	_ = completed
}

func TestRedisQueueRetryBackoffAndDeadLetter(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	store := newFakeRedisStore()
	q := NewRedisQueue(store, RedisQueueOptions{
		QueueName:   "hotkey:test",
		Now:         func() time.Time { return now },
		Backoff:     FixedBackoff(5 * time.Minute),
		MaxAttempts: 2,
	})
	payload := mustPayload(t, GenerateEmbeddingPayload{ItemID: "item-1"})

	enqueued, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeGenerateEmbedding,
		Payload:        payload,
		IdempotencyKey: "embedding:item-1",
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	claimed, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	retried, err := q.Fail(ctx, claimed.ID, NewRetryableError(errTemporary))
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if retried.ID != enqueued.ID || retried.Status != JobStatusScheduled || retried.Attempt != 1 {
		t.Fatalf("unexpected retry state: %+v", retried)
	}
	if !retried.NextRunAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("expected backoff next run, got %s", retried.NextRunAt)
	}
	if _, err := q.Claim(ctx); err != ErrNoJobs {
		t.Fatalf("expected no job before retry enqueue, got %v", err)
	}

	now = now.Add(5 * time.Minute)
	claimed, err = q.Claim(ctx)
	if err != nil {
		t.Fatalf("claim retry failed: %v", err)
	}
	dead, err := q.Fail(ctx, claimed.ID, NewRetryableError(errTemporary))
	if err != nil {
		t.Fatalf("dead letter failed: %v", err)
	}
	if dead.Status != JobStatusDeadLetter || dead.Attempt != 2 {
		t.Fatalf("expected dead letter attempt 2, got %+v", dead)
	}
}
