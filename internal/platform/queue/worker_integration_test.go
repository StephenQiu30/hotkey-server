//go:build integration

package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
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
	if _, _, err := store.Enqueue(ctx, Job{Kind: "worker_fixture", UniqueKey: "worker-success", Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 2, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	called := 0
	worker := NewWorker(runtime, map[string]Handler{"worker_fixture": func(context.Context, Job) error { called++; return nil }})
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
	if _, _, err := store.Enqueue(ctx, Job{Kind: "worker_fixture", UniqueKey: "worker-fail", Payload: Payload{EntityID: 2, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	worker = NewWorker(runtime, map[string]Handler{"worker_fixture": func(context.Context, Job) error { return errors.New("fixture failure") }})
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
