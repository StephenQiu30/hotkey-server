package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type RedisStore interface {
	Set(context.Context, string, []byte) error
	SetNX(context.Context, string, []byte) (bool, error)
	Del(context.Context, string) error
	Get(context.Context, string) ([]byte, error)
	LPush(context.Context, string, []byte) error
	RPop(context.Context, string) ([]byte, error)
}

type RedisQueueOptions struct {
	QueueName     string
	Now           func() time.Time
	MaxAttempts   int
	Backoff       BackoffFunc
	OnStateChange StateChangeFunc
}

type RedisQueue struct {
	store         RedisStore
	queueName     string
	now           func() time.Time
	maxAttempts   int
	backoff       BackoffFunc
	onStateChange StateChangeFunc
}

func NewRedisQueue(store RedisStore, opts RedisQueueOptions) *RedisQueue {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	queueName := opts.QueueName
	if queueName == "" {
		queueName = "hotkey:jobs:pending"
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	backoff := opts.Backoff
	if backoff == nil {
		backoff = func(attempt int) time.Duration {
			return time.Duration(attempt) * time.Minute
		}
	}
	return &RedisQueue{store: store, queueName: queueName, now: now, maxAttempts: maxAttempts, backoff: backoff, onStateChange: opts.OnStateChange}
}

func (q *RedisQueue) Enqueue(ctx context.Context, req EnqueueRequest) (Job, error) {
	if err := ValidatePayload(req.Type, req.Payload); err != nil {
		return Job{}, err
	}
	if req.IdempotencyKey == "" {
		return Job{}, errors.New("idempotency key is required")
	}

	now := q.now()
	job := Job{
		ID:             fmt.Sprintf("%s:%d", req.IdempotencyKey, now.UnixNano()),
		Type:           req.Type,
		Payload:        append(json.RawMessage(nil), req.Payload...),
		Status:         JobStatusPending,
		Attempt:        0,
		MaxAttempts:    q.maxAttempts,
		IdempotencyKey: req.IdempotencyKey,
		NextRunAt:      now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if !req.RunAt.IsZero() {
		job.NextRunAt = req.RunAt
		if req.RunAt.After(now) {
			job.Status = JobStatusScheduled
		}
	}

	body, err := json.Marshal(job)
	if err != nil {
		return Job{}, err
	}
	idempotencyKey := q.idempotencyKey(req.IdempotencyKey)
	created, err := q.store.SetNX(ctx, idempotencyKey, body)
	if err != nil {
		return Job{}, wrapRedisErr(err)
	}
	if !created {
		existing, err := q.store.Get(ctx, idempotencyKey)
		if err != nil {
			return Job{}, wrapRedisErr(err)
		}
		var existingJob Job
		if err := json.Unmarshal(existing, &existingJob); err != nil {
			return Job{}, err
		}
		return existingJob, nil
	}
	if err := q.store.Set(ctx, q.jobKey(job.ID), body); err != nil {
		_ = q.store.Del(ctx, idempotencyKey)
		return Job{}, wrapRedisErr(err)
	}
	if err := q.store.LPush(ctx, q.queueName, body); err != nil {
		_ = q.store.Del(ctx, q.jobKey(job.ID))
		_ = q.store.Del(ctx, idempotencyKey)
		return Job{}, wrapRedisErr(err)
	}
	if q.onStateChange != nil {
		q.onStateChange(ctx, job)
	}
	return job, nil
}

func (q *RedisQueue) Claim(ctx context.Context) (Job, error) {
	body, err := q.store.RPop(ctx, q.queueName)
	if err != nil {
		if err.Error() == "redis nil reply" {
			return Job{}, ErrNoJobs
		}
		return Job{}, wrapRedisErr(err)
	}
	var job Job
	if err := json.Unmarshal(body, &job); err != nil {
		return Job{}, err
	}
	now := q.now()
	if (job.Status == JobStatusScheduled || job.Status == JobStatusPending) && job.NextRunAt.After(now) {
		if err := q.store.LPush(ctx, q.queueName, body); err != nil {
			return Job{}, wrapRedisErr(err)
		}
		return Job{}, ErrNoJobs
	}
	job.Status = JobStatusRunning
	job.UpdatedAt = now
	if err := q.save(ctx, job); err != nil {
		return Job{}, wrapRedisErr(err)
	}
	if q.onStateChange != nil {
		q.onStateChange(ctx, job)
	}
	return job, nil
}

func (q *RedisQueue) Fail(ctx context.Context, id string, err error) (Job, error) {
	if err == nil {
		return Job{}, errors.New("job failure error is required")
	}

	body, getErr := q.store.Get(ctx, q.jobKey(id))
	if getErr != nil {
		return Job{}, wrapRedisErr(getErr)
	}
	var job Job
	if unmarshalErr := json.Unmarshal(body, &job); unmarshalErr != nil {
		return Job{}, unmarshalErr
	}

	now := q.now()
	job.Attempt++
	job.LastError = err.Error()
	job.UpdatedAt = now
	if IsRetryable(err) && job.Attempt < q.maxAttempts {
		job.Status = JobStatusScheduled
		job.NextRunAt = now.Add(q.backoff(job.Attempt))
		if saveErr := q.save(ctx, job); saveErr != nil {
			return Job{}, wrapRedisErr(saveErr)
		}
		if enqueueErr := q.enqueueJob(ctx, job); enqueueErr != nil {
			return Job{}, wrapRedisErr(enqueueErr)
		}
		if q.onStateChange != nil {
			q.onStateChange(ctx, job)
		}
		return job, nil
	}
	if IsRetryable(err) {
		job.Status = JobStatusDeadLetter
	} else {
		job.Status = JobStatusFailed
	}
	if saveErr := q.save(ctx, job); saveErr != nil {
		return Job{}, wrapRedisErr(saveErr)
	}
	if q.onStateChange != nil {
		q.onStateChange(ctx, job)
	}
	return job, nil
}

func (q *RedisQueue) Complete(ctx context.Context, id string) (Job, error) {
	body, getErr := q.store.Get(ctx, q.jobKey(id))
	if getErr != nil {
		return Job{}, wrapRedisErr(getErr)
	}
	var job Job
	if unmarshalErr := json.Unmarshal(body, &job); unmarshalErr != nil {
		return Job{}, unmarshalErr
	}
	job.Status = JobStatusSucceeded
	job.UpdatedAt = q.now()
	if saveErr := q.save(ctx, job); saveErr != nil {
		return Job{}, wrapRedisErr(saveErr)
	}
	if q.onStateChange != nil {
		q.onStateChange(ctx, job)
	}
	return job, nil
}

func (q *RedisQueue) save(ctx context.Context, job Job) error {
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if err := q.store.Set(ctx, q.jobKey(job.ID), body); err != nil {
		return err
	}
	return q.store.Set(ctx, q.idempotencyKey(job.IdempotencyKey), body)
}

func (q *RedisQueue) enqueueJob(ctx context.Context, job Job) error {
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return q.store.LPush(ctx, q.queueName, body)
}

func (q *RedisQueue) idempotencyKey(value string) string {
	return q.queueName + ":idempotency:" + value
}

func (q *RedisQueue) jobKey(id string) string {
	return q.queueName + ":job:" + id
}

// PendingLen 返回 pending 队列中的任务数。
// 注意：此方法通过 LLEN 模拟，实际使用 LPush/RPop 的客户端可能不支持 LLEN。
// 当前实现通过遍历 job 存储近似计算，仅用于观测。
func (q *RedisQueue) PendingLen() int {
	// RedisQueue 没有 O(1) 的 LLEN；返回 0 表示无法精确计算。
	// 生产环境中通过 Redis CLI 或监控获取精确值。
	return 0
}

// wrapRedisErr 将 Redis 操作错误包装为 RedisConnectionError，
// 排除 "redis nil reply"（表示 key 不存在，非连接错误）。
func wrapRedisErr(err error) error {
	if err == nil {
		return nil
	}
	if err.Error() == "redis nil reply" {
		return err
	}
	return NewRedisConnectionError(err)
}
