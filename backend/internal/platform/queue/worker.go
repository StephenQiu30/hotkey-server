package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

type Handler func(context.Context, Job) error

type Worker struct {
	runtime  *database.Runtime
	handlers map[string]Handler
	now      func() time.Time
}

func NewWorker(runtime *database.Runtime, handlers map[string]Handler) *Worker {
	copyHandlers := make(map[string]Handler, len(handlers))
	for kind, handler := range handlers {
		copyHandlers[kind] = handler
	}
	return &Worker{runtime: runtime, handlers: copyHandlers, now: func() time.Time { return time.Now().UTC() }}
}

// RunOnce claims at most one due job. Claim state is committed before the
// handler runs, so a process crash leaves a bounded-attempt job recoverable.
func (worker *Worker) RunOnce(ctx context.Context) (bool, error) {
	if worker == nil || worker.runtime == nil {
		return false, sharedrepository.ErrUnavailable
	}
	var id int64
	var kind string
	var args []byte
	var uniqueKey []byte
	var scheduledAt time.Time
	var attempt, maxAttempts int
	var priority int
	err := worker.runtime.WithinTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		row := transaction.SQL.QueryRowContext(ctx, `SELECT id, kind, args, attempt, max_attempts, priority, scheduled_at, unique_key FROM river_job WHERE state = 'available' AND scheduled_at <= now() ORDER BY priority, id FOR UPDATE SKIP LOCKED LIMIT 1`)
		if err := row.Scan(&id, &kind, &args, &attempt, &maxAttempts, &priority, &scheduledAt, &uniqueKey); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sql.ErrNoRows
			}
			return databaserepository.MapError(err)
		}
		_, err := transaction.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'running', attempt = attempt + 1, attempted_at = now() WHERE id = $1`, id)
		return databaserepository.MapError(err)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	job := Job{ID: id, Kind: kind, UniqueKey: string(uniqueKey), ScheduledAt: scheduledAt, MaxAttempts: maxAttempts, Priority: priority}
	if kindUsesSemanticDurableArgs(kind) {
		job.DurableArgs = append([]byte(nil), args...)
	} else {
		if err := json.Unmarshal(args, &job.Payload); err != nil {
			return true, worker.finish(ctx, id, kind, attempt+1, maxAttempts, NewPermanentError(fmt.Errorf("decode job payload: %w", err)))
		}
	}
	if err := job.Validate(); err != nil {
		return true, worker.finish(ctx, id, kind, attempt+1, maxAttempts, NewPermanentError(err))
	}
	handler := worker.handlers[kind]
	if handler == nil {
		return true, worker.finish(ctx, id, kind, attempt+1, maxAttempts, NewPermanentError(fmt.Errorf("no handler registered for %q", kind)))
	}
	handlerCtx := sharedrequestcontext.WithJobID(ctx, id)
	return true, worker.finish(ctx, id, kind, attempt+1, maxAttempts, handler(handlerCtx, job))
}

func (worker *Worker) finish(ctx context.Context, id int64, kind string, attempt, maxAttempts int, handlerErr error) error {
	return worker.runtime.WithinTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if handlerErr == nil {
			_, err := transaction.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = $1`, id)
			return databaserepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `INSERT INTO river_job_attempt (job_id, attempt, error) VALUES ($1, $2, $3) ON CONFLICT (job_id, attempt) DO NOTHING`, id, attempt, failureCode(handlerErr)); err != nil {
			return databaserepository.MapError(err)
		}
		if IsCancelled(handlerErr) {
			_, err := transaction.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'cancelled', finalized_at = now() WHERE id = $1`, id)
			return databaserepository.MapError(err)
		}
		if IsPermanent(handlerErr) || attempt >= maxAttempts {
			_, err := transaction.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'discarded', finalized_at = now() WHERE id = $1`, id)
			return databaserepository.MapError(err)
		}
		retryAt := worker.now().UTC().Add(retryBackoff(attempt))
		if requested, ok := RetryAt(handlerErr); ok && requested.After(retryAt) {
			retryAt = requested
		}
		_, err := transaction.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'available', scheduled_at = $2, finalized_at = NULL WHERE id = $1`, id, retryAt)
		return databaserepository.MapError(err)
	})
}

// failureCode deliberately drops the wrapped cause. Provider responses,
// query text and credentials must never become durable queue diagnostics.
func failureCode(err error) string {
	switch {
	case IsCancelled(err):
		return "cancelled"
	case IsPermanent(err):
		return "permanent"
	default:
		return "retryable"
	}
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<uint(attempt-1)) * time.Minute
}

func (worker *Worker) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("worker interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := worker.RunOnce(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (worker *Worker) ReclaimStale(ctx context.Context, timeout time.Duration) (int64, error) {
	if worker == nil || worker.runtime == nil {
		return 0, sharedrepository.ErrUnavailable
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("reclaim timeout must be positive")
	}
	var resolved int64
	err := worker.runtime.WithinTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		// A process can exit after the durable claim but before finish records an
		// attempt. Preserve that lease expiry as a bounded, non-sensitive fact.
		if _, err := transaction.SQL.ExecContext(ctx, `
INSERT INTO river_job_attempt (job_id,attempt,error)
SELECT id,attempt,'lease_expired'
FROM river_job
WHERE state='running' AND attempted_at < now() - $1::interval
ON CONFLICT (job_id,attempt) DO NOTHING`, timeout.String()); err != nil {
			return databaserepository.MapError(err)
		}
		requeued, err := transaction.SQL.ExecContext(ctx, `
UPDATE river_job
SET state='available',scheduled_at=now(),finalized_at=NULL
WHERE state='running' AND attempted_at < now() - $1::interval AND attempt < max_attempts`, timeout.String())
		if err != nil {
			return databaserepository.MapError(err)
		}
		discarded, err := transaction.SQL.ExecContext(ctx, `
UPDATE river_job
SET state='discarded',finalized_at=now()
WHERE state='running' AND attempted_at < now() - $1::interval AND attempt >= max_attempts`, timeout.String())
		if err != nil {
			return databaserepository.MapError(err)
		}
		requeuedCount, err := requeued.RowsAffected()
		if err != nil {
			return databaserepository.MapError(err)
		}
		discardedCount, err := discarded.RowsAffected()
		if err != nil {
			return databaserepository.MapError(err)
		}
		resolved = requeuedCount + discardedCount
		return nil
	})
	return resolved, err
}
