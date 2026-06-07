package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestWeeklyEmailSchedulerTickEnqueuesOnMatchingDayAndTime(t *testing.T) {
	producer := &recordingProducer{}
	// Sunday at 09:00
	now := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		ReportID:            "wr-1",
		WeeklySendDay:       time.Sunday,
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklyEnabled: true},
			{UserID: "user-2", EmailEnabled: false, WeeklyEnabled: true},
			{UserID: "user-3", EmailEnabled: true, WeeklyEnabled: false},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 1 {
		t.Fatalf("expected 1 enqueued job (user-2 disabled, user-3 weekly disabled), got %d", len(producer.requests))
	}
	assertWeeklyEmailRequest(t, producer.requests[0], "wr-1", "user-1", "send_weekly_email:wr-1:user-1:2026-W23")
}

func TestWeeklyEmailSchedulerTickSkipsWrongDay(t *testing.T) {
	producer := &recordingProducer{}
	// Monday at 09:00 (scheduler expects Sunday)
	now := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		ReportID:            "wr-1",
		WeeklySendDay:       time.Sunday,
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklyEnabled: true},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 0 {
		t.Fatalf("expected 0 enqueued jobs on wrong day, got %d", len(producer.requests))
	}
}

func TestWeeklyEmailSchedulerTickSkipsWrongTime(t *testing.T) {
	producer := &recordingProducer{}
	// Sunday at 10:00 (scheduler expects 09:00)
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		ReportID:            "wr-1",
		WeeklySendDay:       time.Sunday,
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklyEnabled: true},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 0 {
		t.Fatalf("expected 0 enqueued jobs at wrong time, got %d", len(producer.requests))
	}
}

func TestWeeklyEmailSchedulerTickSkipsWhenNoReportID(t *testing.T) {
	producer := &recordingProducer{}
	now := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		// No ReportID and no ReportIDProvider
		WeeklySendDay:       time.Sunday,
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklyEnabled: true},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 0 {
		t.Fatalf("expected 0 enqueued jobs when no report ID, got %d", len(producer.requests))
	}
}

func TestWeeklyEmailSchedulerTickUsesReportIDProvider(t *testing.T) {
	producer := &recordingProducer{}
	now := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	providerCalled := false
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		ReportIDProvider: func() string {
			providerCalled = true
			return "wr-dynamic"
		},
		WeeklySendDay:       time.Sunday,
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklyEnabled: true},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if !providerCalled {
		t.Fatal("expected reportIDProvider to be called")
	}
	if len(producer.requests) != 1 {
		t.Fatalf("expected 1 enqueued job, got %d", len(producer.requests))
	}
	assertWeeklyEmailRequest(t, producer.requests[0], "wr-dynamic", "user-1", "send_weekly_email:wr-dynamic:user-1:2026-W23")
}

func TestWeeklyEmailSchedulerTickPerRecipientSendAt(t *testing.T) {
	producer := &recordingProducer{}
	// Sunday at 09:00
	now := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		ReportID:            "wr-1",
		WeeklySendDay:       time.Sunday,
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklyEnabled: true, WeeklySendAt: "09:00"},
			{UserID: "user-2", EmailEnabled: true, WeeklyEnabled: true, WeeklySendAt: "10:00"},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 1 {
		t.Fatalf("expected 1 enqueued job (user-2 at 10:00), got %d", len(producer.requests))
	}
	// Verify correct recipient
	var payload queue.SendWeeklyEmailPayload
	if err := json.Unmarshal(producer.requests[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if payload.RecipientUserID != "user-1" {
		t.Fatalf("expected recipient user-1, got %q", payload.RecipientUserID)
	}
}

func assertWeeklyEmailRequest(t *testing.T, req queue.EnqueueRequest, reportID string, userID string, idempotencyKey string) {
	t.Helper()
	if req.Type != queue.JobTypeSendWeeklyEmail {
		t.Fatalf("expected send_weekly_email job, got %s", req.Type)
	}
	if req.IdempotencyKey != idempotencyKey {
		t.Fatalf("unexpected idempotency key %q", req.IdempotencyKey)
	}
	var payload queue.SendWeeklyEmailPayload
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		t.Fatalf("payload was not send_weekly_email payload: %v", err)
	}
	if payload.ReportID != reportID || payload.RecipientUserID != userID {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
