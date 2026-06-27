package jobs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
)

func TestRunnerExecutesJobAtLeastOnce(t *testing.T) {
	var count atomic.Int32
	r := jobs.NewRunner()
	r.Register("test-job", func(ctx context.Context) error {
		count.Add(1)
		return nil
	}, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	r.Run(ctx)

	if count.Load() < 1 {
		t.Fatal("expected job to execute at least once")
	}
}

func TestRunnerStopsOnContextCancel(t *testing.T) {
	var count atomic.Int32
	r := jobs.NewRunner()
	r.Register("cancel-job", func(ctx context.Context) error {
		count.Add(1)
		return nil
	}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("runner did not stop after context cancel")
	}

	final := count.Load()
	if final < 1 {
		t.Fatal("expected at least one execution before stop")
	}
}

func TestRunnerHandlesJobError(t *testing.T) {
	var count atomic.Int32
	r := jobs.NewRunner()
	r.Register("error-job", func(ctx context.Context) error {
		count.Add(1)
		return context.DeadlineExceeded
	}, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	r.Run(ctx)

	if count.Load() < 1 {
		t.Fatal("expected job to run despite errors")
	}
}

func TestRunnerRecordsLifecycleWithRunKey(t *testing.T) {
	recorder := &recordingRunRecorder{}
	r := jobs.NewRunner(jobs.WithRunRecorder(recorder))
	ctx, cancel := context.WithCancel(context.Background())
	r.Register("audited-job", func(ctx context.Context) error {
		cancel()
		return nil
	}, time.Hour, jobs.WithRunKey(func(_ time.Time) string {
		return "audited-job:2026-06-26"
	}))

	r.Run(ctx)

	if len(recorder.events) != 2 {
		t.Fatalf("expected start and success audit events, got %d", len(recorder.events))
	}
	started := recorder.events[0]
	if started.JobName != "audited-job" || started.RunKey != "audited-job:2026-06-26" || started.Status != jobs.RunStatusStarted {
		t.Fatalf("unexpected start event: %+v", started)
	}
	succeeded := recorder.events[1]
	if succeeded.Status != jobs.RunStatusSucceeded || succeeded.Attempt != 1 || succeeded.Error != "" {
		t.Fatalf("unexpected success event: %+v", succeeded)
	}
}

func TestRunnerRetriesFailuresAndAuditsFinalError(t *testing.T) {
	recorder := &recordingRunRecorder{}
	var attempts atomic.Int32
	r := jobs.NewRunner(
		jobs.WithRunRecorder(recorder),
		jobs.WithRetryPolicy(jobs.RetryPolicy{MaxAttempts: 2}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	r.Register("retry-job", func(ctx context.Context) error {
		if attempts.Add(1) == 2 {
			cancel()
		}
		return errors.New("upstream unavailable")
	}, time.Hour, jobs.WithRunKey(func(_ time.Time) string {
		return "retry-job:fixed"
	}))

	r.Run(ctx)

	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts.Load())
	}
	if len(recorder.events) != 2 {
		t.Fatalf("expected start and final failure audit events, got %d", len(recorder.events))
	}
	failed := recorder.events[1]
	if failed.Status != jobs.RunStatusFailed {
		t.Fatalf("expected failed status, got %+v", failed)
	}
	if failed.Attempt != 2 {
		t.Fatalf("expected failed audit to record final attempt 2, got %d", failed.Attempt)
	}
	if failed.Error != "upstream unavailable" {
		t.Fatalf("expected failure reason, got %q", failed.Error)
	}
	if failed.RunKey != "retry-job:fixed" {
		t.Fatalf("expected stable run key, got %q", failed.RunKey)
	}
}

func TestRunnerAuditsCancellationDuringRetryBackoff(t *testing.T) {
	recorder := &recordingRunRecorder{}
	r := jobs.NewRunner(
		jobs.WithRunRecorder(recorder),
		jobs.WithRetryPolicy(jobs.RetryPolicy{MaxAttempts: 2, Backoff: time.Hour}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	r.Register("cancelled-retry-job", func(ctx context.Context) error {
		cancel()
		return errors.New("temporary failure")
	}, time.Hour, jobs.WithRunKey(func(_ time.Time) string {
		return "cancelled-retry-job:fixed"
	}))

	r.Run(ctx)

	if len(recorder.events) != 2 {
		t.Fatalf("expected start and cancelled failure audit events, got %d", len(recorder.events))
	}
	failed := recorder.events[1]
	if failed.Status != jobs.RunStatusFailed {
		t.Fatalf("expected failed status, got %+v", failed)
	}
	if failed.Attempt != 1 {
		t.Fatalf("expected cancellation after first attempt, got attempt %d", failed.Attempt)
	}
	if failed.Error != context.Canceled.Error() {
		t.Fatalf("expected context cancellation reason, got %q", failed.Error)
	}
}

func TestRunnerHonorsCancellationBetweenZeroBackoffRetries(t *testing.T) {
	recorder := &recordingRunRecorder{}
	var attempts atomic.Int32
	r := jobs.NewRunner(
		jobs.WithRunRecorder(recorder),
		jobs.WithRetryPolicy(jobs.RetryPolicy{MaxAttempts: 3}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	r.Register("cancelled-zero-backoff-job", func(ctx context.Context) error {
		attempts.Add(1)
		cancel()
		return errors.New("temporary failure")
	}, time.Hour, jobs.WithRunKey(func(_ time.Time) string {
		return "cancelled-zero-backoff-job:fixed"
	}))

	r.Run(ctx)

	if attempts.Load() != 1 {
		t.Fatalf("expected cancellation to stop zero-backoff retries after 1 attempt, got %d", attempts.Load())
	}
	if len(recorder.events) != 2 {
		t.Fatalf("expected start and cancelled failure audit events, got %d", len(recorder.events))
	}
	failed := recorder.events[1]
	if failed.Status != jobs.RunStatusFailed || failed.Attempt != 1 || failed.Error != context.Canceled.Error() {
		t.Fatalf("unexpected cancellation audit event: %+v", failed)
	}
}

func TestRunnerBoundsSuccessfulRunKeyCache(t *testing.T) {
	var attempts atomic.Int32
	var keyCalls atomic.Int32
	r := jobs.NewRunner(
		jobs.WithRunRecorder(nil),
		jobs.WithSuccessCacheLimit(1),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	r.Register("bounded-key-job", func(ctx context.Context) error {
		if attempts.Add(1) == 3 {
			cancel()
		}
		return nil
	}, time.Millisecond, jobs.WithRunKey(func(_ time.Time) string {
		switch keyCalls.Add(1) {
		case 1:
			return "bounded-key-job:a"
		case 2:
			return "bounded-key-job:b"
		default:
			return "bounded-key-job:a"
		}
	}))

	r.Run(ctx)

	if attempts.Load() != 3 {
		t.Fatalf("expected old successful run key to be evicted and run again, got %d attempts", attempts.Load())
	}
}

func TestRunnerPropagatesStableRunStartedAtAcrossRetries(t *testing.T) {
	var attempts atomic.Int32
	var first time.Time
	r := jobs.NewRunner(jobs.WithRetryPolicy(jobs.RetryPolicy{MaxAttempts: 2}))
	ctx, cancel := context.WithCancel(context.Background())
	r.Register("started-at-job", func(ctx context.Context) error {
		startedAt, ok := jobs.RunStartedAtFromContext(ctx)
		if !ok || startedAt.IsZero() {
			t.Fatal("expected run started time in job context")
		}
		if attempts.Add(1) == 1 {
			first = startedAt
			return errors.New("retry me")
		}
		cancel()
		if !startedAt.Equal(first) {
			t.Fatalf("expected stable run started time across retries, got %s then %s", first, startedAt)
		}
		return nil
	}, time.Hour)

	r.Run(ctx)
}

func TestRunnerSkipsDuplicateSuccessfulRunKeyInSingleInstance(t *testing.T) {
	recorder := &recordingRunRecorder{}
	var attempts atomic.Int32
	r := jobs.NewRunner(jobs.WithRunRecorder(recorder))
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	r.Register("daily-job", func(ctx context.Context) error {
		attempts.Add(1)
		return nil
	}, 10*time.Millisecond, jobs.WithRunKey(func(_ time.Time) string {
		return "daily-job:2026-06-26"
	}))

	r.Run(ctx)

	if attempts.Load() != 1 {
		t.Fatalf("expected duplicate successful run key to execute once, got %d", attempts.Load())
	}
	if len(recorder.events) < 3 {
		t.Fatalf("expected start, success, and skipped audit events, got %d", len(recorder.events))
	}
	if recorder.events[2].Status != jobs.RunStatusSkipped {
		t.Fatalf("expected duplicate run to be audited as skipped, got %+v", recorder.events[2])
	}
}

func TestRunnerDoesNotStartJobWhenContextAlreadyCancelled(t *testing.T) {
	var attempts atomic.Int32
	r := jobs.NewRunner(jobs.WithRunRecorder(nil))
	r.Register("cancelled-before-start", func(ctx context.Context) error {
		attempts.Add(1)
		return nil
	}, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.Run(ctx)

	if attempts.Load() != 0 {
		t.Fatalf("expected no attempts after context cancellation, got %d", attempts.Load())
	}
}

func TestLogRunRecorderWritesTraceableJSON(t *testing.T) {
	var out bytes.Buffer
	recorder := jobs.NewLogRunRecorder(&out)

	err := recorder.RecordJobRun(context.Background(), jobs.RunEvent{
		JobName:    "publish_daily_topics",
		RunKey:     "publish_daily_topics:2026-06-26",
		Status:     jobs.RunStatusFailed,
		Attempt:    3,
		StartedAt:  time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 6, 26, 8, 0, 2, 0, time.UTC),
		Duration:   2 * time.Second,
		Error:      "llm timeout",
	})
	if err != nil {
		t.Fatalf("record event: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON audit log, got %v: %s", err, out.String())
	}

	assertJSONField(t, body, "module", "worker")
	assertJSONField(t, body, "job", "publish_daily_topics")
	assertJSONField(t, body, "run_key", "publish_daily_topics:2026-06-26")
	assertJSONField(t, body, "status", "failed")
	assertJSONField(t, body, "error", "llm timeout")
	if got := body["attempt"]; got != float64(3) {
		t.Fatalf("expected attempt 3, got %#v", got)
	}
	if got := body["duration_ms"]; got != float64(2000) {
		t.Fatalf("expected duration_ms 2000, got %#v", got)
	}
}

type recordingRunRecorder struct {
	events []jobs.RunEvent
}

func (r *recordingRunRecorder) RecordJobRun(_ context.Context, event jobs.RunEvent) error {
	r.events = append(r.events, event)
	return nil
}

func assertJSONField(t *testing.T, body map[string]any, key, want string) {
	t.Helper()
	if got := body[key]; got != want {
		t.Fatalf("expected %s=%q, got %#v", key, want, got)
	}
}
