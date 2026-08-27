//go:build integration

package queue

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

const (
	queueCrashAfterClaimFlag = "HOTKEY_TEST_QUEUE_CRASH_AFTER_CLAIM"
	queueCrashDSNFlag        = "HOTKEY_TEST_QUEUE_CRASH_DSN"
	queueCrashExitCode       = 91
)

func TestWorkerKillAfterClaimReclaimsLeaseAndAppliesSideEffectOnce(t *testing.T) {
	ctx := context.Background()
	dsn := postgresfixture.New(t)
	runtime, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
CREATE TABLE worker_recovery_effects (
  job_id bigint PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(runtime)
	jobID, created, err := store.Enqueue(ctx, Job{
		Kind: KindRunRetention, UniqueKey: "worker-crash-after-claim",
		Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC().Add(-time.Minute),
		MaxAttempts: 3, Priority: 1,
	})
	if err != nil || !created {
		t.Fatalf("Enqueue() = %d/%t/%v", jobID, created, err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestWorkerCrashAfterClaimHelper$")
	command.Env = append(os.Environ(), queueCrashAfterClaimFlag+"=1", queueCrashDSNFlag+"="+dsn)
	output, err := command.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != queueCrashExitCode {
		t.Fatalf("crash worker exit = %v, output=%s", err, output)
	}

	var state string
	var attempt int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state,attempt FROM river_job WHERE id=$1`, jobID).Scan(&state, &attempt); err != nil {
		t.Fatal(err)
	}
	if state != "running" || attempt != 1 {
		t.Fatalf("post-crash claim = %q attempt %d, want running/1", state, attempt)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET attempted_at=now()-interval '2 minutes' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}

	recovered := NewWorker(runtime, map[string]Handler{KindRunRetention: func(ctx context.Context, job Job) error {
		_, err := runtime.SQL.ExecContext(ctx, `INSERT INTO worker_recovery_effects (job_id) VALUES ($1) ON CONFLICT DO NOTHING`, job.ID)
		return err
	}})
	if reclaimed, err := recovered.ReclaimStale(ctx, time.Minute); err != nil || reclaimed != 1 {
		t.Fatalf("ReclaimStale() = %d/%v, want 1/nil", reclaimed, err)
	}
	if worked, err := recovered.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("RunOnce(recovered) = %t/%v", worked, err)
	}

	var effects, leaseFailures int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state,attempt FROM river_job WHERE id=$1`, jobID).Scan(&state, &attempt); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM worker_recovery_effects WHERE job_id=$1`, jobID).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job_attempt WHERE job_id=$1 AND error='lease_expired'`, jobID).Scan(&leaseFailures); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || attempt != 2 || effects != 1 || leaseFailures != 1 {
		t.Fatalf("recovered facts = state %q attempt %d effects %d lease_failures %d", state, attempt, effects, leaseFailures)
	}
}

func TestWorkerCrashAfterClaimHelper(t *testing.T) {
	if os.Getenv(queueCrashAfterClaimFlag) != "1" {
		return
	}
	runtime, err := database.Open(context.Background(), os.Getenv(queueCrashDSNFlag))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	worker := NewWorker(runtime, map[string]Handler{KindRunRetention: func(context.Context, Job) error {
		os.Exit(queueCrashExitCode)
		return nil
	}})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(crash helper) = %t/%v", worked, err)
	}
	t.Fatal("crash helper returned without terminating")
}
