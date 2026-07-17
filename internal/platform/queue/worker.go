package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
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
	var attempt, maxAttempts int
	err := worker.runtime.WithinTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		row := transaction.SQL.QueryRowContext(ctx, `SELECT id, kind, args, attempt, max_attempts FROM river_job WHERE state = 'available' AND scheduled_at <= now() ORDER BY priority, id FOR UPDATE SKIP LOCKED LIMIT 1`)
		if err := row.Scan(&id, &kind, &args, &attempt, &maxAttempts); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sql.ErrNoRows
			}
			return sharedrepository.MapError(err)
		}
		_, err := transaction.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'running', attempt = attempt + 1, attempted_at = now() WHERE id = $1`, id)
		return sharedrepository.MapError(err)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var payload Payload
	if err := json.Unmarshal(args, &payload); err != nil {
		return true, worker.finish(ctx, id, kind, attempt+1, maxAttempts, fmt.Errorf("decode job payload: %w", err))
	}
	job := Job{ID: id, Kind: kind, Payload: payload, ScheduledAt: worker.now()}
	handler := worker.handlers[kind]
	if handler == nil {
		return true, worker.finish(ctx, id, kind, attempt+1, maxAttempts, fmt.Errorf("no handler registered for %q", kind))
	}
	return true, worker.finish(ctx, id, kind, attempt+1, maxAttempts, handler(ctx, job))
}

func (worker *Worker) finish(ctx context.Context, id int64, kind string, attempt, maxAttempts int, handlerErr error) error {
	return worker.runtime.WithinTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if handlerErr == nil {
			_, err := transaction.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = $1`, id)
			return sharedrepository.MapError(err)
		}
		state := "available"
		if attempt >= maxAttempts {
			state = "discarded"
		}
		if _, err := transaction.SQL.ExecContext(ctx, `INSERT INTO river_job_attempt (job_id, attempt, error) VALUES ($1, $2, $3) ON CONFLICT (job_id, attempt) DO NOTHING`, id, attempt, handlerErr.Error()); err != nil {
			return sharedrepository.MapError(err)
		}
		_, err := transaction.SQL.ExecContext(ctx, `UPDATE river_job SET state = $1, scheduled_at = now() + interval '1 minute', finalized_at = CASE WHEN $1 = 'discarded' THEN now() ELSE NULL END WHERE id = $2`, state, id)
		return sharedrepository.MapError(err)
	})
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
	result, err := worker.runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'available', scheduled_at = now() WHERE state = 'running' AND attempted_at < now() - $1::interval AND attempt < max_attempts`, timeout.String())
	if err != nil {
		return 0, sharedrepository.MapError(err)
	}
	return result.RowsAffected()
}
