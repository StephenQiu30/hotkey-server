//go:build integration

package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestWorkerClaimsCompletesAndRetriesJobs(t *testing.T) {
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
	if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: "worker-success", Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 2, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	called := 0
	worker := NewWorker(runtime, map[string]Handler{KindRunRetention: func(context.Context, Job) error { called++; return nil }})
	claimed, err := worker.RunOnce(ctx)
	if err != nil || !claimed || called != 1 {
		t.Fatalf("successful RunOnce() = %v/%v, calls=%d", claimed, err, called)
	}
	var state string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE unique_key = $1`, []byte("worker-success")).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "completed" {
		t.Fatalf("successful job state = %q", state)
	}
	if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: "worker-fail", Payload: Payload{EntityID: 2, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	worker = NewWorker(runtime, map[string]Handler{KindRunRetention: func(context.Context, Job) error { return errors.New("fixture failure") }})
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE unique_key = $1`, []byte("worker-fail")).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "discarded" {
		t.Fatalf("failed job state = %q, want discarded", state)
	}
}

func TestWorkerClassifiesPermanentAndCancelledFailures(t *testing.T) {
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
	for _, key := range []string{"worker-permanent", "worker-cancelled"} {
		if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: key, Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 1}); err != nil {
			t.Fatal(err)
		}
	}
	worker := NewWorker(runtime, map[string]Handler{KindRunRetention: func(_ context.Context, job Job) error {
		if job.UniqueKey == "worker-permanent" {
			return NewPermanentError(errors.New("invalid fixture"))
		}
		return NewCancelledError(context.Canceled)
	}})
	for range 2 {
		if claimed, err := worker.RunOnce(ctx); err != nil || !claimed {
			t.Fatalf("RunOnce() = %v/%v", claimed, err)
		}
	}
	for _, test := range []struct {
		key, want string
	}{
		{key: "worker-permanent", want: "discarded"},
		{key: "worker-cancelled", want: "cancelled"},
	} {
		var state string
		if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE unique_key = $1`, []byte(test.key)).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != test.want {
			t.Fatalf("job %q state = %q, want %q", test.key, state, test.want)
		}
	}
}

func TestWorkerReclaimsExpiredRunningJobs(t *testing.T) {
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
	if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: "worker-stale", Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'running', attempt = 1, attempted_at = now() - interval '2 minutes' WHERE unique_key = $1`, []byte("worker-stale")); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(runtime, nil)
	reclaimed, err := worker.ReclaimStale(ctx, time.Minute)
	if err != nil || reclaimed != 1 {
		t.Fatalf("ReclaimStale() = %d/%v, want 1/nil", reclaimed, err)
	}
	var state string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE unique_key = $1`, []byte("worker-stale")).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "available" {
		t.Fatalf("reclaimed state = %q, want available", state)
	}
}
