package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestDailyEmailSchedulerUsesDefaultTimeAndSkipsDisabledUsers(t *testing.T) {
	ctx := context.Background()
	producer := &recordingProducer{}
	now := time.Date(2026, 5, 31, 8, 30, 0, 0, time.UTC)
	scheduler := NewDailyEmailScheduler(producer, DailyEmailOptions{
		ReportID:           "report-1",
		DefaultDailySendAt: "08:30",
		Recipients: []DailyEmailRecipient{
			{UserID: "user-enabled", EmailEnabled: true},
			{UserID: "user-disabled", EmailEnabled: false},
		},
		Now: func() time.Time { return now },
	})

	if err := scheduler.Tick(ctx); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 1 {
		t.Fatalf("expected only enabled user to be enqueued, got %d requests", len(producer.requests))
	}
	assertSendDailyEmailRequest(t, producer.requests[0], "report-1", "user-enabled", "send_daily_email:report-1:user-enabled:2026-05-31")
}

func TestDailyEmailSchedulerCustomTimeOverridesDefault(t *testing.T) {
	ctx := context.Background()
	producer := &recordingProducer{}
	now := time.Date(2026, 5, 31, 9, 15, 0, 0, time.UTC)
	scheduler := NewDailyEmailScheduler(producer, DailyEmailOptions{
		ReportID:           "report-1",
		DefaultDailySendAt: "08:30",
		Recipients: []DailyEmailRecipient{
			{UserID: "default-user", EmailEnabled: true},
			{UserID: "custom-user", EmailEnabled: true, DailySendAt: "09:15"},
		},
		Now: func() time.Time { return now },
	})

	if err := scheduler.Tick(ctx); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 1 {
		t.Fatalf("expected only custom-time user to be enqueued, got %d requests", len(producer.requests))
	}
	assertSendDailyEmailRequest(t, producer.requests[0], "report-1", "custom-user", "send_daily_email:report-1:custom-user:2026-05-31")
}

func assertSendDailyEmailRequest(t *testing.T, req queue.EnqueueRequest, reportID string, userID string, idempotencyKey string) {
	t.Helper()
	if req.Type != queue.JobTypeSendDailyEmail {
		t.Fatalf("expected send_daily_email job, got %s", req.Type)
	}
	if req.IdempotencyKey != idempotencyKey {
		t.Fatalf("unexpected idempotency key %q", req.IdempotencyKey)
	}
	var payload queue.SendDailyEmailPayload
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		t.Fatalf("payload was not send_daily_email payload: %v", err)
	}
	if payload.ReportID != reportID || payload.RecipientUserID != userID {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
