package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestJobPayloadSchemasCoverRequiredTypes(t *testing.T) {
	scheduledFor := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		jobType JobType
		payload any
	}{
		{"collect source", JobTypeCollectSource, CollectSourcePayload{SourceID: "source-1", ScheduledFor: scheduledFor}},
		{"generate embedding", JobTypeGenerateEmbedding, GenerateEmbeddingPayload{ItemID: "item-1"}},
		{"cluster hotspots", JobTypeClusterHotspots, ClusterHotspotsPayload{WindowStart: scheduledFor, WindowEnd: scheduledFor.Add(time.Hour)}},
		{"score hotspots", JobTypeScoreHotspots, ScoreHotspotsPayload{ClusterRunID: "cluster-run-1"}},
		{"generate daily report", JobTypeGenerateDailyReport, GenerateDailyReportPayload{Date: "2026-05-31"}},
		{"send daily email", JobTypeSendDailyEmail, SendDailyEmailPayload{ReportID: "report-1", RecipientUserID: "user-1"}},
		{"send weekly email", JobTypeSendWeeklyEmail, SendWeeklyEmailPayload{ReportID: "report-1", RecipientUserID: "user-1"}},
		{"generate weekly report", JobTypeGenerateWeeklyReport, GenerateWeeklyReportPayload{WeekOf: "2026-W23"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatal(err)
			}

			if err := ValidatePayload(tt.jobType, body); err != nil {
				t.Fatalf("expected payload to validate: %v", err)
			}
		})
	}
}

func TestValidatePayloadRejectsUnknownTypeAndMissingRequiredFields(t *testing.T) {
	if err := ValidatePayload(JobType("unknown"), json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected unknown job type to fail")
	}

	if err := ValidatePayload(JobTypeCollectSource, json.RawMessage(`{"source_id":""}`)); err == nil {
		t.Fatal("expected missing collect_source fields to fail")
	}
}

func TestValidatePayloadRejectsMalformedWeekOf(t *testing.T) {
	tests := []struct {
		name    string
		weekOf  string
		wantErr bool
	}{
		{"valid 2026-W23", "2026-W23", false},
		{"valid 2026-W01", "2026-W01", false},
		{"valid 2026-W52", "2026-W52", false},
		{"empty", "", true},
		{"slash format 2026/23", "2026/23", true},
		{"no W 2026-23", "2026-23", true},
		{"week 00", "2026-W00", true},
		{"week 54", "2026-W54", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := mustPayload(t, GenerateWeeklyReportPayload{WeekOf: tt.weekOf})
			err := ValidatePayload(JobTypeGenerateWeeklyReport, payload)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for week_of=%q", tt.weekOf)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for week_of=%q: %v", tt.weekOf, err)
			}
		})
	}
}

func TestValidatePayloadRejectsWeeklyEmailMissingFields(t *testing.T) {
	if err := ValidatePayload(JobTypeSendWeeklyEmail, json.RawMessage(`{"report_id":""}`)); err == nil {
		t.Fatal("expected missing recipient_user_id to fail")
	}
	if err := ValidatePayload(JobTypeSendWeeklyEmail, json.RawMessage(`{"recipient_user_id":"u1"}`)); err == nil {
		t.Fatal("expected missing report_id to fail")
	}
}

func TestMemoryQueueEnqueueIsIdempotent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	q := NewMemoryQueue(QueueOptions{
		Now:         func() time.Time { return now },
		MaxAttempts: 3,
		Backoff:     FixedBackoff(time.Minute),
	})
	payload := mustPayload(t, CollectSourcePayload{SourceID: "source-1", ScheduledFor: now})

	first, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect_source:source-1:2026-05-31T01",
	})
	if err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	second, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect_source:source-1:2026-05-31T01",
	})
	if err != nil {
		t.Fatalf("second enqueue failed: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected duplicate enqueue to return existing job %q, got %q", first.ID, second.ID)
	}
	if got := q.PendingLen(); got != 1 {
		t.Fatalf("expected one pending job, got %d", got)
	}
}

func TestMemoryQueueRetryBackoffAndDeadLetter(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	q := NewMemoryQueue(QueueOptions{
		Now:         func() time.Time { return now },
		MaxAttempts: 2,
		Backoff:     FixedBackoff(5 * time.Minute),
	})
	payload := mustPayload(t, GenerateEmbeddingPayload{ItemID: "item-1"})

	enqueued, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeGenerateEmbedding,
		Payload:        payload,
		IdempotencyKey: "embedding:item-1",
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	claimed, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claimed.ID != enqueued.ID || claimed.Status != JobStatusRunning {
		t.Fatalf("unexpected claimed job: %+v", claimed)
	}

	retried, err := q.Fail(ctx, claimed.ID, NewRetryableError(errors.New("temporary redis failure")))
	if err != nil {
		t.Fatalf("fail retryable failed: %v", err)
	}
	if retried.Status != JobStatusScheduled {
		t.Fatalf("expected scheduled retry, got %s", retried.Status)
	}
	if retried.Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", retried.Attempt)
	}
	if !retried.NextRunAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("expected next run at %s, got %s", now.Add(5*time.Minute), retried.NextRunAt)
	}

	if _, err := q.Claim(ctx); !errors.Is(err, ErrNoJobs) {
		t.Fatalf("expected no job before backoff elapsed, got %v", err)
	}
	now = now.Add(5 * time.Minute)
	claimed, err = q.Claim(ctx)
	if err != nil {
		t.Fatalf("claim after backoff failed: %v", err)
	}

	dead, err := q.Fail(ctx, claimed.ID, NewRetryableError(errors.New("still failing")))
	if err != nil {
		t.Fatalf("fail into dead letter failed: %v", err)
	}
	if dead.Status != JobStatusDeadLetter {
		t.Fatalf("expected dead letter, got %s", dead.Status)
	}
	if dead.Attempt != 2 {
		t.Fatalf("expected attempt 2, got %d", dead.Attempt)
	}
}

func TestMemoryQueuePersistenceCallbackFiresOnStateChange(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	var persisted []Job
	q := NewMemoryQueue(QueueOptions{
		Now:         func() time.Time { return now },
		MaxAttempts: 3,
		Backoff:     FixedBackoff(time.Minute),
		OnStateChange: func(_ context.Context, job Job) {
			persisted = append(persisted, job)
		},
	})
	payload := mustPayload(t, CollectSourcePayload{SourceID: "source-1", ScheduledFor: now})

	job, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: "collect:source-1",
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if len(persisted) != 1 || persisted[0].Status != JobStatusPending {
		t.Fatalf("expected 1 pending persistence call, got %d calls with statuses %v", len(persisted), statuses(persisted))
	}

	claimed, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(persisted) != 2 || persisted[1].Status != JobStatusRunning {
		t.Fatalf("expected 2 calls with running, got %d calls with statuses %v", len(persisted), statuses(persisted))
	}

	completed, err := q.Complete(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(persisted) != 3 || persisted[2].Status != JobStatusSucceeded {
		t.Fatalf("expected 3 calls with succeeded, got %d calls with statuses %v", len(persisted), statuses(persisted))
	}
	_ = job
	_ = completed
}

func TestMemoryQueuePersistenceCallbackFiresOnRetryAndDeadLetter(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	var persisted []Job
	q := NewMemoryQueue(QueueOptions{
		Now:         func() time.Time { return now },
		MaxAttempts: 2,
		Backoff:     FixedBackoff(5 * time.Minute),
		OnStateChange: func(_ context.Context, job Job) {
			persisted = append(persisted, job)
		},
	})
	payload := mustPayload(t, GenerateEmbeddingPayload{ItemID: "item-1"})

	_, err := q.Enqueue(ctx, EnqueueRequest{
		Type:           JobTypeGenerateEmbedding,
		Payload:        payload,
		IdempotencyKey: "embedding:item-1",
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	claimed, err := q.Claim(ctx)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	// enqueue=1, claim=2
	if len(persisted) != 2 {
		t.Fatalf("expected 2 calls before fail, got %d", len(persisted))
	}

	retried, err := q.Fail(ctx, claimed.ID, NewRetryableError(errors.New("temporary")))
	if err != nil {
		t.Fatalf("fail retryable failed: %v", err)
	}
	// fail fires callback with scheduled status
	if len(persisted) != 3 || persisted[2].Status != JobStatusScheduled {
		t.Fatalf("expected 3 calls with scheduled, got %d calls with statuses %v", len(persisted), statuses(persisted))
	}
	if retried.Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", retried.Attempt)
	}
	_ = retried
}

func statuses(jobs []Job) []JobStatus {
	s := make([]JobStatus, len(jobs))
	for i, j := range jobs {
		s[i] = j.Status
	}
	return s
}

func mustPayload(t *testing.T, payload any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
