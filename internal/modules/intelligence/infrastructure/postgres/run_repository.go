package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

// Transition is the persistence-only state machine. It refreshes every
// in-flight lease atomically and never calls a provider while holding locks.
func (repository *Repository) Transition(ctx context.Context, runID int64, target intelligencedomain.RunStatus, now time.Time) (intelligencedomain.Run, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || runID <= 0 || !target.Valid() {
		return intelligencedomain.Run{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if !inFlight(target) {
		return intelligencedomain.Run{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	initial, err := repository.runLifecycle(ctx, repository.queryer(ctx), runID, false)
	if err != nil {
		return intelligencedomain.Run{}, err
	}
	var transitioned intelligencedomain.Run
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockBudget(ctx, transaction.SQL, initial.ModelProfileID, initial.BudgetDay); err != nil {
			return err
		}
		if err := lockRun(ctx, transaction.SQL, initial.ReuseKey); err != nil {
			return err
		}
		locked, err := repository.runLifecycle(ctx, transaction.SQL, runID, true)
		if err != nil {
			return err
		}
		if locked.runReference != initial.runReference || !intelligencedomain.CanTransition(locked.Status, target) {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		if target == intelligencedomain.RunStatusRetryWait && locked.Attempt >= locked.MaxAttempts {
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
		}
		if target == intelligencedomain.RunStatusRunning && locked.Status == intelligencedomain.RunStatusRetryWait && locked.RetryAfter.Valid && now.Before(locked.RetryAfter.Time.UTC()) {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}

		attempt := locked.Attempt
		lease := now.Add(time.Duration(locked.TimeoutSeconds+30) * time.Second)
		var retryAfter any
		if target == intelligencedomain.RunStatusRetryWait {
			delay := retryDelay(locked.Attempt)
			retry := now.Add(delay)
			retryAfter = retry
			lease = retry.Add(time.Duration(locked.TimeoutSeconds+30) * time.Second)
			attempt++
		}
		if err := transaction.SQL.QueryRowContext(ctx, `
UPDATE ai_runs
SET status=$1::varchar, attempt=$2, retry_after=$3, lease_expires_at=$4,
	started_at=CASE WHEN $1::varchar='running' THEN COALESCE(started_at,$5) ELSE started_at END
WHERE id=$6
RETURNING id,task_type,target_type,target_id,model_profile_id,model_profile_version,model_version,reuse_key,status,
		  structured_result,tokens,latency_ms,reserved_cost::text,cost::text,error_code,lease_expires_at`,
			string(target), attempt, retryAfter, lease, now, runID,
		).Scan(runScanTargets(&transitioned)...); err != nil {
			return fmt.Errorf("transition AI run: %w", err)
		}
		return nil
	})
	return transitioned, err
}

// Cancel releases an unspent reservation. Completed cost is only written by
// Settle, never by a cancellation path.
func (repository *Repository) Cancel(ctx context.Context, runID int64, now time.Time) (intelligencedomain.Run, error) {
	released, _, err := repository.releaseInFlight(ctx, runID, now, intelligencedomain.RunStatusCancelled, nil, false)
	return released, err
}

// Fail makes a permanent, already-classified failure terminal and releases its
// reservation. Successful work is settled separately because it owns the only
// path that writes actual provider cost.
func (repository *Repository) Fail(ctx context.Context, runID int64, errorCode int, now time.Time) (intelligencedomain.Run, error) {
	if errorCode < intelligencedomain.CodeAIModelProfileInvalid || errorCode > intelligencedomain.CodeAIRunLeaseExpired {
		return intelligencedomain.Run{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	released, _, err := repository.releaseInFlight(ctx, runID, now, intelligencedomain.RunStatusFailed, intPointer(errorCode), false)
	return released, err
}

// ReclaimExpired is safe for a worker loop: it marks only expired in-flight
// rows, releases their reservation, and never replays provider work.
func (repository *Repository) ReclaimExpired(ctx context.Context, now time.Time) (int, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return 0, fmt.Errorf("AI run repository is unavailable")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	rows, err := repository.queryRows(ctx, `
SELECT id FROM ai_runs
WHERE status IN ('queued','running','validating','retry_wait') AND lease_expires_at < $1
ORDER BY model_profile_id,budget_day,id`, now)
	if err != nil {
		return 0, fmt.Errorf("list expired AI runs: %w", err)
	}
	var runIDs []int64
	for rows.Next() {
		var runID int64
		if err := rows.Scan(&runID); err != nil {
			return 0, fmt.Errorf("scan expired AI run: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate expired AI runs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close expired AI runs: %w", err)
	}

	reclaimed := 0
	for _, runID := range runIDs {
		_, changed, err := repository.releaseInFlight(ctx, runID, now, intelligencedomain.RunStatusFailed, intPointer(intelligencedomain.CodeAIRunLeaseExpired), true)
		if err != nil {
			return reclaimed, err
		}
		if changed {
			reclaimed++
		}
	}
	return reclaimed, nil
}

func (repository *Repository) releaseInFlight(ctx context.Context, runID int64, now time.Time, terminal intelligencedomain.RunStatus, errorCode *int, onlyExpired bool) (intelligencedomain.Run, bool, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || runID <= 0 ||
		(terminal != intelligencedomain.RunStatusCancelled && terminal != intelligencedomain.RunStatusFailed) {
		return intelligencedomain.Run{}, false, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	initial, err := repository.runLifecycle(ctx, repository.queryer(ctx), runID, false)
	if err != nil {
		return intelligencedomain.Run{}, false, err
	}
	var released intelligencedomain.Run
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockBudget(ctx, transaction.SQL, initial.ModelProfileID, initial.BudgetDay); err != nil {
			return err
		}
		if err := lockRun(ctx, transaction.SQL, initial.ReuseKey); err != nil {
			return err
		}
		locked, err := repository.runLifecycle(ctx, transaction.SQL, runID, true)
		if err != nil {
			return err
		}
		if locked.ModelProfileID != initial.ModelProfileID || locked.BudgetDay != initial.BudgetDay || locked.ReuseKey != initial.ReuseKey {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		if !inFlight(locked.Status) {
			if !onlyExpired {
				return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
			}
			// An overlapping worker already made the row terminal.
			released = runFromLifecycle(locked)
			return nil
		}
		if onlyExpired && (locked.LeaseExpiresAt == nil || !locked.LeaseExpiresAt.Before(now)) {
			// The row became live again after it was listed by the worker.
			released = runFromLifecycle(locked)
			return nil
		}
		if err := releaseBudget(ctx, transaction.SQL, locked.runReference); err != nil {
			return err
		}
		var code any
		if errorCode != nil {
			code = *errorCode
		}
		if err := transaction.SQL.QueryRowContext(ctx, `
UPDATE ai_runs
SET status=$1,reserved_cost=0,error_code=$2,lease_expires_at=NULL,finished_at=$3
WHERE id=$4
RETURNING id,task_type,target_type,target_id,model_profile_id,model_profile_version,model_version,reuse_key,status,
		  structured_result,tokens,latency_ms,reserved_cost::text,cost::text,error_code,lease_expires_at`,
			string(terminal), code, now, runID,
		).Scan(runScanTargets(&released)...); err != nil {
			return fmt.Errorf("release AI run reservation: %w", err)
		}
		return nil
	})
	return released, err == nil && released.ID != 0 && released.Status == terminal, err
}

type runLifecycle struct {
	runReference
	TimeoutSeconds, Attempt, MaxAttempts int64
	RetryAfter                           sql.NullTime
	RepairAttempted                      bool
	LeaseExpiresAt                       *time.Time
}

func (repository *Repository) runLifecycle(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, runID int64, lock bool) (runLifecycle, error) {
	return readRunLifecycle(ctx, queryer, runID, lock)
}

func readRunLifecycle(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, runID int64, lock bool) (runLifecycle, error) {
	query := `SELECT r.model_profile_id,r.budget_day::text,r.reuse_key,r.reserved_cost::text,r.status,
       p.timeout_seconds,r.attempt,r.max_attempts,r.retry_after,r.repair_attempted,r.lease_expires_at
FROM ai_runs r JOIN ai_model_profiles p ON p.id=r.model_profile_id WHERE r.id=$1`
	if lock {
		query += " FOR UPDATE OF r"
	}
	var lifecycle runLifecycle
	var lease sql.NullTime
	if err := queryer.QueryRowContext(ctx, query, runID).Scan(
		&lifecycle.ModelProfileID, &lifecycle.BudgetDay, &lifecycle.ReuseKey, &lifecycle.ReservedCost, &lifecycle.Status,
		&lifecycle.TimeoutSeconds, &lifecycle.Attempt, &lifecycle.MaxAttempts, &lifecycle.RetryAfter, &lifecycle.RepairAttempted, &lease,
	); err != nil {
		if err == sql.ErrNoRows {
			return runLifecycle{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		return runLifecycle{}, fmt.Errorf("read AI run lifecycle: %w", err)
	}
	if lease.Valid {
		value := lease.Time.UTC()
		lifecycle.LeaseExpiresAt = &value
	}
	return lifecycle, nil
}

// BeginRepair records the one permitted structured-output repair before a
// second provider request. It refreshes the validating lease under the same
// budget -> run advisory order and refuses a second repair after a retry or
// process restart.
func (repository *Repository) BeginRepair(ctx context.Context, runID int64, now time.Time) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || runID <= 0 {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	initial, err := repository.runLifecycle(ctx, repository.queryer(ctx), runID, false)
	if err != nil {
		return err
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockBudget(ctx, transaction.SQL, initial.ModelProfileID, initial.BudgetDay); err != nil {
			return err
		}
		if err := lockRun(ctx, transaction.SQL, initial.ReuseKey); err != nil {
			return err
		}
		locked, err := repository.runLifecycle(ctx, transaction.SQL, runID, true)
		if err != nil {
			return err
		}
		if locked.runReference != initial.runReference || locked.Status != intelligencedomain.RunStatusValidating || locked.RepairAttempted {
			return intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
		}
		lease := now.Add(time.Duration(locked.TimeoutSeconds+30) * time.Second)
		result, err := transaction.SQL.ExecContext(ctx, `
UPDATE ai_runs
SET repair_attempted=true,lease_expires_at=$1
WHERE id=$2 AND repair_attempted=false`, lease, runID)
		if err != nil {
			return fmt.Errorf("record AI output repair: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read AI output repair result: %w", err)
		}
		if affected != 1 {
			return intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
		}
		return nil
	})
}

func releaseBudget(ctx context.Context, queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, reference runReference) error {
	result, err := queryer.ExecContext(ctx, `
UPDATE ai_budget_ledgers
SET reserved_cost=reserved_cost-$1::numeric,updated_at=now()
WHERE model_profile_id=$2 AND budget_day=$3 AND reserved_cost >= $1::numeric`,
		reference.ReservedCost, reference.ModelProfileID, reference.BudgetDay)
	if err != nil {
		return fmt.Errorf("release AI budget: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read AI budget release result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("AI budget reservation is inconsistent")
	}
	return nil
}

// reclaimExpiredForBudget runs only after the caller holds the profile/day
// advisory lock. Each candidate then obtains its run lock, preserving the
// single budget -> run order used by claims, transitions and the worker.
func reclaimExpiredForBudget(ctx context.Context, transaction *sql.Tx, profileID int64, budgetDay string, now time.Time) (int, error) {
	rows, err := transaction.QueryContext(ctx, `
SELECT id FROM ai_runs
WHERE model_profile_id=$1 AND budget_day=$2
  AND status IN ('queued','running','validating','retry_wait')
  AND lease_expires_at < $3
ORDER BY id`, profileID, budgetDay, now)
	if err != nil {
		return 0, fmt.Errorf("list profile-day expired AI runs: %w", err)
	}
	var runIDs []int64
	for rows.Next() {
		var runID int64
		if err := rows.Scan(&runID); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan profile-day expired AI run: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate profile-day expired AI runs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close profile-day expired AI runs: %w", err)
	}

	reclaimed := 0
	for _, runID := range runIDs {
		lifecycle, err := readRunLifecycle(ctx, transaction, runID, false)
		if err != nil {
			return reclaimed, err
		}
		if err := lockRun(ctx, transaction, lifecycle.ReuseKey); err != nil {
			return reclaimed, err
		}
		locked, err := readRunLifecycle(ctx, transaction, runID, true)
		if err != nil {
			return reclaimed, err
		}
		if !inFlight(locked.Status) || locked.LeaseExpiresAt == nil || !locked.LeaseExpiresAt.Before(now) {
			continue
		}
		if err := releaseBudget(ctx, transaction, locked.runReference); err != nil {
			return reclaimed, err
		}
		if _, err := transaction.ExecContext(ctx, `
UPDATE ai_runs
SET status='failed',reserved_cost=0,error_code=$1,lease_expires_at=NULL,finished_at=$2
WHERE id=$3`, intelligencedomain.CodeAIRunLeaseExpired, now, runID); err != nil {
			return reclaimed, fmt.Errorf("reclaim expired AI run: %w", err)
		}
		reclaimed++
	}
	return reclaimed, nil
}

func retryDelay(attempt int64) time.Duration {
	seconds := int64(1)
	for current := int64(1); current < attempt && seconds < 4; current++ {
		seconds *= 2
	}
	if seconds > 4 {
		seconds = 4
	}
	return time.Duration(seconds) * time.Second
}

func runFromLifecycle(lifecycle runLifecycle) intelligencedomain.Run {
	return intelligencedomain.Run{
		ModelProfileID: lifecycle.ModelProfileID, ReuseKey: lifecycle.ReuseKey, Status: lifecycle.Status,
		ReservedCost: lifecycle.ReservedCost, LeaseExpiresAt: lifecycle.LeaseExpiresAt,
	}
}

func intPointer(value int) *int { return &value }
