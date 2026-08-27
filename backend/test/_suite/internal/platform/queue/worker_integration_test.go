//go:build integration

package queue

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestWorkerRoundTripsSemanticDurableArgsWithoutGenericPayload(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"evidence_reference_id":71,"trace_id":"0123456789abcdef0123456789abcdef"}`)
	store := NewStore(runtime)
	if _, _, err := store.Enqueue(ctx, Job{
		Kind: KindGenerateSourceDocument, UniqueKey: "source-document-71", DurableArgs: args,
		ScheduledAt: dueWorkerFixtureTime(), MaxAttempts: 3, Priority: 3,
	}); err != nil {
		t.Fatal(err)
	}
	var received Job
	var owningJobID int64
	worker := NewWorker(runtime, map[string]Handler{KindGenerateSourceDocument: func(ctx context.Context, job Job) error {
		received = job
		owningJobID = sharedrequestcontext.JobID(ctx)
		return nil
	}})
	if claimed, err := worker.RunOnce(ctx); err != nil || !claimed {
		t.Fatalf("RunOnce() = %t/%v", claimed, err)
	}
	if !sameJSONObject(t, received.DurableArgs, args) || received.Payload != (Payload{}) {
		t.Fatalf("received durable job = %#v", received)
	}
	if owningJobID <= 0 || owningJobID != received.ID {
		t.Fatalf("owning job context = %d, job ID = %d", owningJobID, received.ID)
	}
	var stored []byte
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT args FROM river_job WHERE unique_key=$1`, []byte("source-document-71")).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !sameJSONObject(t, stored, args) {
		t.Fatalf("stored args = %s", stored)
	}
}

func sameJSONObject(t *testing.T, left, right []byte) bool {
	t.Helper()
	var leftValue, rightValue map[string]any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		t.Fatalf("decode left JSON object: %v", err)
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		t.Fatalf("decode right JSON object: %v", err)
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

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
	if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: "worker-success", Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: dueWorkerFixtureTime(), MaxAttempts: 2, Priority: 1}); err != nil {
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
	if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: "worker-fail", Payload: Payload{EntityID: 2, EntityVersion: 1}, ScheduledAt: dueWorkerFixtureTime(), MaxAttempts: 1, Priority: 1}); err != nil {
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

func TestWorkerSchedulesRetryWithExponentialBackoff(t *testing.T) {
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
	if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: "worker-backoff", Payload: Payload{EntityID: 3, EntityVersion: 1}, ScheduledAt: dueWorkerFixtureTime(), MaxAttempts: 3, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 8, 1, 2, 3, 0, time.UTC)
	worker := NewWorker(runtime, map[string]Handler{KindRunRetention: func(context.Context, Job) error { return errors.New("temporary fixture failure") }})
	worker.now = func() time.Time { return now }
	if claimed, err := worker.RunOnce(ctx); err != nil || !claimed {
		t.Fatalf("first RunOnce() = %v/%v", claimed, err)
	}
	assertWorkerRetrySchedule(t, runtime, "worker-backoff", 1, now.Add(time.Minute))
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET scheduled_at=now() WHERE unique_key=$1`, []byte("worker-backoff")); err != nil {
		t.Fatal(err)
	}
	if claimed, err := worker.RunOnce(ctx); err != nil || !claimed {
		t.Fatalf("second RunOnce() = %v/%v", claimed, err)
	}
	assertWorkerRetrySchedule(t, runtime, "worker-backoff", 2, now.Add(2*time.Minute))
}

func TestWorkerNeverRetriesBeforeProviderReset(t *testing.T) {
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
	if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: "worker-provider-reset", Payload: Payload{EntityID: 4, EntityVersion: 1}, ScheduledAt: dueWorkerFixtureTime(), MaxAttempts: 3, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC)
	resetAt := now.Add(15 * time.Minute)
	worker := NewWorker(runtime, map[string]Handler{KindRunRetention: func(context.Context, Job) error {
		return NewRetryableErrorAt(errors.New("provider rate limit"), resetAt)
	}})
	worker.now = func() time.Time { return now }
	if claimed, err := worker.RunOnce(ctx); err != nil || !claimed {
		t.Fatalf("RunOnce() = %t/%v", claimed, err)
	}
	assertWorkerRetrySchedule(t, runtime, "worker-provider-reset", 1, resetAt)
}

func assertWorkerRetrySchedule(t *testing.T, runtime *database.Runtime, key string, wantAttempt int, wantScheduledAt time.Time) {
	t.Helper()
	var state string
	var attempt int
	var scheduledAt time.Time
	if err := runtime.SQL.QueryRowContext(context.Background(), `SELECT state,attempt,scheduled_at FROM river_job WHERE unique_key=$1`, []byte(key)).Scan(&state, &attempt, &scheduledAt); err != nil {
		t.Fatal(err)
	}
	if state != "available" || attempt != wantAttempt || !scheduledAt.Equal(wantScheduledAt) {
		t.Fatalf("retry = state %q attempt %d scheduled %s, want available/%d/%s", state, attempt, scheduledAt, wantAttempt, wantScheduledAt)
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
		if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: key, Payload: Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: dueWorkerFixtureTime(), MaxAttempts: 3, Priority: 1}); err != nil {
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

func TestWorkerRecoversExpiredClaimsAndTerminatesExhaustedLease(t *testing.T) {
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
	if _, _, err := store.Enqueue(ctx, Job{Kind: KindRunRetention, UniqueKey: "worker-stale-exhausted", Payload: Payload{EntityID: 2, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'running', attempt = 1, attempted_at = now() - interval '2 minutes' WHERE unique_key = $1`, []byte("worker-stale")); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'running', attempt = 1, attempted_at = now() - interval '2 minutes' WHERE unique_key = $1`, []byte("worker-stale-exhausted")); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(runtime, nil)
	reclaimed, err := worker.ReclaimStale(ctx, time.Minute)
	if err != nil || reclaimed != 2 {
		t.Fatalf("ReclaimStale() = %d/%v, want 2/nil", reclaimed, err)
	}
	var recoverableState, exhaustedState string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE unique_key = $1`, []byte("worker-stale")).Scan(&recoverableState); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE unique_key = $1`, []byte("worker-stale-exhausted")).Scan(&exhaustedState); err != nil {
		t.Fatal(err)
	}
	if recoverableState != "available" || exhaustedState != "discarded" {
		t.Fatalf("reclaimed states = %q/%q, want available/discarded", recoverableState, exhaustedState)
	}
	var leaseFailures int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job_attempt WHERE error='lease_expired'`).Scan(&leaseFailures); err != nil {
		t.Fatal(err)
	}
	if leaseFailures != 2 {
		t.Fatalf("lease-expiry attempt facts = %d, want 2", leaseFailures)
	}
	second, err := worker.ReclaimStale(ctx, time.Minute)
	if err != nil || second != 0 {
		t.Fatalf("idempotent ReclaimStale() = %d/%v, want 0/nil", second, err)
	}
}
