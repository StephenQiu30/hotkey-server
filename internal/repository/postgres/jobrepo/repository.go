package jobrepo

import (
	"context"
	"database/sql"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, job queue.Job) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO jobs (
			id,
			job_type,
			payload,
			status,
			attempt,
			max_attempts,
			idempotency_key,
			last_error,
			scheduled_at,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, job.ID, string(job.Type), []byte(job.Payload), string(job.Status), job.Attempt, job.MaxAttempts, job.IdempotencyKey, job.LastError, job.NextRunAt, job.CreatedAt, job.UpdatedAt)
	return err
}

func (r *Repository) FindByIdempotencyKey(ctx context.Context, key string) (queue.Job, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id,
			job_type,
			payload,
			status,
			attempt,
			max_attempts,
			idempotency_key,
			last_error,
			scheduled_at,
			created_at,
			updated_at
		FROM jobs
		WHERE idempotency_key = $1
	`, key)

	var job queue.Job
	var jobType string
	var status string
	var payload []byte
	if err := row.Scan(&job.ID, &jobType, &payload, &status, &job.Attempt, &job.MaxAttempts, &job.IdempotencyKey, &job.LastError, &job.NextRunAt, &job.CreatedAt, &job.UpdatedAt); err != nil {
		return queue.Job{}, err
	}
	job.Type = queue.JobType(jobType)
	job.Payload = append(job.Payload[:0], payload...)
	job.Status = queue.JobStatus(status)
	return job, nil
}

// UpdateStatus 更新 job 的状态、失败原因、尝试次数和更新时间。
// 用于 queue 状态变更后的 PostgreSQL 持久化。
func (r *Repository) UpdateStatus(ctx context.Context, id string, status queue.JobStatus, lastError string, attempt int) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = $1,
		    last_error = $2,
		    attempt = $3,
		    updated_at = $4
		WHERE id = $5
	`, string(status), lastError, attempt, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
