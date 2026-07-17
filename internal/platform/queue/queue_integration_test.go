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
