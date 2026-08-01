//go:build integration

package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestEnqueueUsesStableKindAndKey(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	store := NewStore(runtime)
	job := Job{Kind: "collect_source", UniqueKey: "stable-key", Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 1}
	firstID, firstCreated, err := store.Enqueue(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	secondID, secondCreated, err := store.Enqueue(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if firstID == 0 || firstID != secondID || !firstCreated || secondCreated {
		t.Fatalf("enqueue = %d/%t, %d/%t", firstID, firstCreated, secondID, secondCreated)
	}
}

func TestReactivateByUniqueKeyPreservesAttemptHistory(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	store := NewStore(runtime)
	job := Job{Kind: KindCollectSource, UniqueKey: "retry-history", Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 1}
	id, _, err := store.Enqueue(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'discarded', attempt = 2, finalized_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO river_job_attempt (job_id, attempt, error) VALUES ($1, 1, 'first'), ($1, 2, 'second')`, id); err != nil {
		t.Fatal(err)
	}

	activatedID, err := store.ReactivateByUniqueKey(ctx, job.Kind, job.UniqueKey)
	if err != nil || activatedID != id {
		t.Fatalf("ReactivateByUniqueKey() = %d/%v", activatedID, err)
	}
	var state string
	var attempt, maxAttempts, history int
	var finalized *time.Time
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state, attempt, max_attempts, finalized_at FROM river_job WHERE id = $1`, id).Scan(&state, &attempt, &maxAttempts, &finalized); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job_attempt WHERE job_id = $1`, id).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if state != "available" || attempt != 2 || maxAttempts != 4 || finalized != nil || history != 2 {
		t.Fatalf("reactivated job = %s attempt=%d max=%d finalized=%v history=%d", state, attempt, maxAttempts, finalized, history)
	}
}

func TestReactivateByUniqueKeyRejectsUnsafeStatesAndRollsBack(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	store := NewStore(runtime)
	job := Job{Kind: KindCollectSource, UniqueKey: "retry-conflict", Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 1}
	id, _, err := store.Enqueue(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"running", "completed"} {
		if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state = $1 WHERE id = $2`, state, id); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ReactivateByUniqueKey(ctx, job.Kind, job.UniqueKey); !errors.Is(err, sharedrepository.ErrConflict) {
			t.Fatalf("state %s error = %v, want conflict", state, err)
		}
	}
	if _, err := store.ReactivateByUniqueKey(ctx, job.Kind, "missing"); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("missing error = %v, want not found", err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'cancelled', finalized_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	err = runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		if _, err := store.ReactivateByUniqueKey(transactionCtx, job.Kind, job.UniqueKey); err != nil {
			return err
		}
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("rollback error = %v", err)
	}
	var state string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE id = $1`, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "cancelled" {
		t.Fatalf("rolled back state = %q", state)
	}
}

func TestEnqueueParticipatesInCallerTransaction(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	store := NewStore(runtime)
	job := Job{Kind: "normalize_content", UniqueKey: "transactional-key", Payload: Payload{EntityID: 2, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 1}
	err = runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		if _, _, err := store.Enqueue(transactionCtx, job); err != nil {
			return err
		}
		return context.Canceled
	})
	if err == nil {
		t.Fatal("transaction unexpectedly committed")
	}
	var count int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE kind = $1 AND unique_key = $2`, job.Kind, []byte(job.UniqueKey)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled back job count = %d, want 0", count)
	}
}
