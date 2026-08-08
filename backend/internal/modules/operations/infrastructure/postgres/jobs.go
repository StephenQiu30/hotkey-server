package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ operationsapplication.JobStore = (*JobRepository)(nil)
var _ operationsapplication.JobStoreWithHook = (*JobRepository)(nil)

type JobRepository struct{ runtime *database.Runtime }

const safeResourceIDProjection = `CASE WHEN args->>'entity_id' ~ '^[1-9][0-9]{0,18}$'
THEN CASE WHEN (args->>'entity_id')::numeric <= 9223372036854775807 THEN (args->>'entity_id')::bigint ELSE 0 END
ELSE 0 END`

func NewJobRepository(runtime *database.Runtime) *JobRepository {
	return &JobRepository{runtime: runtime}
}

func (repository *JobRepository) ListJobs(ctx context.Context, query operationsdomain.JobListQuery) (operationsdomain.JobPage, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return operationsdomain.JobPage{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return operationsdomain.JobPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	args := []any{query.Cursor, query.Limit + 1}
	filters := []string{"id > $1"}
	if query.Kind != "" {
		args = append(args, query.Kind)
		filters = append(filters, fmt.Sprintf("kind = $%d", len(args)))
	}
	if query.State != "" {
		args = append(args, string(query.State))
		filters = append(filters, fmt.Sprintf("state = $%d", len(args)))
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `SELECT id, kind, `+safeResourceIDProjection+`,
state, attempt, max_attempts, priority, scheduled_at, attempted_at, finalized_at, created_at,
COALESCE((SELECT CASE WHEN attempt.error IN ('retryable','permanent','cancelled') THEN attempt.error ELSE '' END
          FROM river_job_attempt attempt WHERE attempt.job_id=river_job.id ORDER BY attempt.attempt DESC LIMIT 1),'')
FROM river_job WHERE `+strings.Join(filters, " AND ")+` ORDER BY id ASC LIMIT $2`, args...)
	if err != nil {
		return operationsdomain.JobPage{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	page := operationsdomain.JobPage{Items: make([]operationsdomain.JobSummary, 0, query.Limit)}
	for rows.Next() {
		job, err := scanJobSummary(rows)
		if err != nil {
			return operationsdomain.JobPage{}, err
		}
		if len(page.Items) == query.Limit {
			page.NextCursor = job.ID
			break
		}
		page.Items = append(page.Items, job)
	}
	if err := rows.Err(); err != nil {
		return operationsdomain.JobPage{}, databaserepository.MapError(err)
	}
	return page, nil
}

func (repository *JobRepository) CancelJob(ctx context.Context, jobID int64) (operationsdomain.JobSummary, error) {
	return repository.CancelJobWithHook(ctx, jobID, nil)
}

func (repository *JobRepository) CancelJobWithHook(ctx context.Context, jobID int64, hook func(context.Context, operationsdomain.JobSummary) error) (operationsdomain.JobSummary, error) {
	return repository.mutateJob(ctx, jobID, "cancelled", "available", hook)
}

func (repository *JobRepository) RetryJob(ctx context.Context, jobID int64) (operationsdomain.JobSummary, error) {
	return repository.RetryJobWithHook(ctx, jobID, nil)
}

func (repository *JobRepository) RetryJobWithHook(ctx context.Context, jobID int64, hook func(context.Context, operationsdomain.JobSummary) error) (operationsdomain.JobSummary, error) {
	return repository.mutateJob(ctx, jobID, "available", "discarded', 'cancelled", hook)
}

func (repository *JobRepository) mutateJob(ctx context.Context, jobID int64, nextState, allowedStates string, hook func(context.Context, operationsdomain.JobSummary) error) (operationsdomain.JobSummary, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return operationsdomain.JobSummary{}, sharedrepository.ErrUnavailable
	}
	if jobID <= 0 {
		return operationsdomain.JobSummary{}, fmt.Errorf("%w: positive job id is required", sharedrepository.ErrInvalidInput)
	}
	var result operationsdomain.JobSummary
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		var query string
		args := []any{nextState, jobID}
		if nextState == "cancelled" {
			query = `UPDATE river_job SET state = $1, finalized_at = now() WHERE id = $2 AND state = 'available' RETURNING id, kind, ` + safeResourceIDProjection + `, state, attempt, max_attempts, priority, scheduled_at, attempted_at, finalized_at, created_at, ''`
		} else {
			query = `UPDATE river_job SET state = $1, attempt = 0, attempted_at = NULL, finalized_at = NULL, scheduled_at = now(), errors = ARRAY[]::jsonb[] WHERE id = $2 AND state IN ('discarded', 'cancelled') RETURNING id, kind, ` + safeResourceIDProjection + `, state, attempt, max_attempts, priority, scheduled_at, attempted_at, finalized_at, created_at, ''`
		}
		if _, err := scanJobSummary(transaction.SQL.QueryRowContext(transactionCtx, query, args...), &result); err != nil {
			if err == sql.ErrNoRows {
				return repository.classifyMutationConflict(transactionCtx, transaction.SQL, jobID, allowedStates)
			}
			return databaserepository.MapError(err)
		}
		if hook != nil {
			if err := hook(transactionCtx, result); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return operationsdomain.JobSummary{}, err
	}
	return result, nil
}

func (repository *JobRepository) classifyMutationConflict(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, jobID int64, allowedStates string) error {
	var state string
	if err := query.QueryRowContext(ctx, `SELECT state FROM river_job WHERE id = $1`, jobID).Scan(&state); err == sql.ErrNoRows {
		return fmt.Errorf("%w: job %d", sharedrepository.ErrNotFound, jobID)
	} else if err != nil {
		return databaserepository.MapError(err)
	}
	return fmt.Errorf("%w: job %d is %s; expected %s", sharedrepository.ErrConflict, jobID, state, allowedStates)
}

type rowScanner interface{ Scan(...any) error }

func scanJobSummary(row rowScanner, target ...*operationsdomain.JobSummary) (operationsdomain.JobSummary, error) {
	var job operationsdomain.JobSummary
	var state string
	var attempted, finalized sql.NullTime
	if err := row.Scan(&job.ID, &job.Kind, &job.ResourceID, &state, &job.Attempt, &job.MaxAttempts, &job.Priority, &job.ScheduledAt, &attempted, &finalized, &job.CreatedAt, &job.FailureCode); err != nil {
		return operationsdomain.JobSummary{}, databaserepository.MapError(err)
	}
	job.State = operationsdomain.JobState(state)
	if attempted.Valid {
		value := attempted.Time
		job.AttemptedAt = &value
	}
	if finalized.Valid {
		value := finalized.Time
		job.FinalizedAt = &value
	}
	if err := job.Validate(); err != nil {
		return operationsdomain.JobSummary{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	if len(target) > 0 && target[0] != nil {
		*target[0] = job
	}
	return job, nil
}
