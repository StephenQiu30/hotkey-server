package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestWorkerPersistsOnlyStableFailureCodes(t *testing.T) {
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
	jobID, _, err := store.Enqueue(ctx, Job{
		Kind: KindRunRetention, UniqueKey: "worker-safe-error", Payload: Payload{EntityID: 1, EntityVersion: 1},
		ScheduledAt: time.Now().UTC(), MaxAttempts: 2, Priority: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(runtime, map[string]Handler{KindRunRetention: func(context.Context, Job) error {
		return errors.New("upstream secret token=do-not-store query=private-topic")
	}})
	if claimed, err := worker.RunOnce(ctx); err != nil || !claimed {
		t.Fatalf("RunOnce() = %t/%v", claimed, err)
	}
	var failureCode string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT error FROM river_job_attempt WHERE job_id=$1`, jobID).Scan(&failureCode); err != nil {
		t.Fatal(err)
	}
	if failureCode != "retryable" || strings.Contains(failureCode, "secret") || strings.Contains(failureCode, "private-topic") {
		t.Fatalf("persisted failure = %q, want stable retryable code", failureCode)
	}
}

func TestFailureCodeClassifiesWithoutPreservingTheCause(t *testing.T) {
	secret := errors.New("provider token=do-not-store")
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "retryable", err: secret, want: "retryable"},
		{name: "permanent", err: NewPermanentError(secret), want: "permanent"},
		{name: "cancelled", err: NewCancelledError(secret), want: "cancelled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := failureCode(test.err)
			if got != test.want || strings.Contains(got, "do-not-store") {
				t.Fatalf("failureCode() = %q, want %q without cause", got, test.want)
			}
		})
	}
}
