//go:build integration

package queue

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
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
