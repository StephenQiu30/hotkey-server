package jobrepo

import (
	"context"
	"database/sql"

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
