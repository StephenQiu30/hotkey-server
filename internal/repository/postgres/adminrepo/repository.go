package adminrepo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/service/admin"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateAuditLog(ctx context.Context, entry admin.AuditLog) (admin.AuditLog, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id,
			actor_id,
			action,
			resource_type,
			resource_id,
			result,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, entry.ID, entry.ActorID, entry.Action, entry.ResourceType, entry.ResourceID, entry.Result, entry.CreatedAt)
	return entry, err
}

func (r *Repository) ListAuditLogs(ctx context.Context) (_ []admin.AuditLog, err error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, actor_id, action, resource_type, resource_id, result, created_at
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var logs []admin.AuditLog
	for rows.Next() {
		var entry admin.AuditLog
		if err := rows.Scan(&entry.ID, &entry.ActorID, &entry.Action, &entry.ResourceType, &entry.ResourceID, &entry.Result, &entry.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *Repository) CreateJob(ctx context.Context, job queue.Job) (queue.Job, error) {
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
	return job, err
}

func (r *Repository) ListJobs(ctx context.Context) (_ []queue.Job, err error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, job_type, payload, status, attempt, max_attempts, idempotency_key, last_error, scheduled_at, created_at, updated_at
		FROM jobs
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var jobs []queue.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *Repository) JobByID(ctx context.Context, jobID string) (queue.Job, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, job_type, payload, status, attempt, max_attempts, idempotency_key, last_error, scheduled_at, created_at, updated_at
		FROM jobs
		WHERE id = $1
	`, jobID)
	return scanJob(row)
}

func (r *Repository) UpdateJob(ctx context.Context, job queue.Job) (queue.Job, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = $2,
			attempt = $3,
			last_error = $4,
			scheduled_at = $5,
			updated_at = $6
		WHERE id = $1
	`, job.ID, string(job.Status), job.Attempt, job.LastError, job.NextRunAt, job.UpdatedAt)
	if err != nil {
		return queue.Job{}, err
	}
	if count, err := result.RowsAffected(); err != nil {
		return queue.Job{}, err
	} else if count == 0 {
		return queue.Job{}, admin.ErrNotFound
	}
	return job, nil
}

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJob(scanner jobScanner) (queue.Job, error) {
	var job queue.Job
	var jobType string
	var status string
	var payload []byte
	if err := scanner.Scan(&job.ID, &jobType, &payload, &status, &job.Attempt, &job.MaxAttempts, &job.IdempotencyKey, &job.LastError, &job.NextRunAt, &job.CreatedAt, &job.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return queue.Job{}, admin.ErrNotFound
		}
		return queue.Job{}, err
	}
	job.Type = queue.JobType(jobType)
	job.Status = queue.JobStatus(status)
	job.Payload = append(job.Payload[:0], payload...)
	return job, nil
}

func (r *Repository) UserByID(ctx context.Context, userID string) (admin.UserRecord, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, email FROM users WHERE id = $1`, userID)
	var u admin.UserRecord
	if err := row.Scan(&u.ID, &u.Email); err != nil {
		if err == sql.ErrNoRows {
			return admin.UserRecord{}, admin.ErrNotFound
		}
		return admin.UserRecord{}, err
	}
	return u, nil
}

func (r *Repository) DeleteUser(ctx context.Context, userID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return admin.ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteRSSFeedByUser(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM rss_feeds WHERE user_id = $1`, userID)
	return err
}

func (r *Repository) DeleteDailyReportsByUser(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM daily_reports WHERE user_id = $1`, userID)
	return err
}

func (r *Repository) SaveCleanupTask(ctx context.Context, task admin.CleanupTask) error {
	stepsJSON, err := json.Marshal(task.Steps)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO cleanup_tasks (id, user_id, status, steps, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET status = $3, steps = $4, updated_at = $6
	`, task.ID, task.UserID, string(task.Status), stepsJSON, task.CreatedAt, task.UpdatedAt)
	return err
}

func (r *Repository) CleanupTaskByID(ctx context.Context, taskID string) (admin.CleanupTask, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, status, steps, created_at, updated_at
		FROM cleanup_tasks WHERE id = $1
	`, taskID)
	var task admin.CleanupTask
	var status string
	var stepsJSON []byte
	if err := row.Scan(&task.ID, &task.UserID, &status, &stepsJSON, &task.CreatedAt, &task.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return admin.CleanupTask{}, admin.ErrNotFound
		}
		return admin.CleanupTask{}, err
	}
	task.Status = admin.CleanupStatus(status)
	if err := json.Unmarshal(stepsJSON, &task.Steps); err != nil {
		return admin.CleanupTask{}, err
	}
	return task, nil
}
