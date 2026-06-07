package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestWeeklyEmailSchedulerTickEnqueuesOnMatchingDayAndTime(t *testing.T) {
	producer := &fakeProducer{}
	// Sunday at 09:00
	now := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		ReportID:           "wr-1",
		WeeklySendDay:      time.Sunday,
		WeeklySendAt:       "09:00",
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklySendAt: ""},
			{UserID: "user-2", EmailEnabled: false, WeeklySendAt: ""},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.enqueued) != 1 {
		t.Fatalf("expected 1 enqueued job (user-2 disabled), got %d", len(producer.enqueued))
	}
	if producer.enqueued[0].Type != queue.JobTypeSendWeeklyEmail {
		t.Fatalf("expected send_weekly_email job type, got %s", producer.enqueued[0].Type)
	}
}

func TestWeeklyEmailSchedulerTickSkipsWrongDay(t *testing.T) {
	producer := &fakeProducer{}
	// Monday at 09:00 (scheduler expects Sunday)
	now := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		ReportID:           "wr-1",
		WeeklySendDay:      time.Sunday,
		WeeklySendAt:       "09:00",
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklySendAt: ""},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.enqueued) != 0 {
		t.Fatalf("expected 0 enqueued jobs on wrong day, got %d", len(producer.enqueued))
	}
}

func TestWeeklyEmailSchedulerTickSkipsWrongTime(t *testing.T) {
	producer := &fakeProducer{}
	// Sunday at 10:00 (scheduler expects 09:00)
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	s := NewWeeklyEmailScheduler(producer, WeeklyEmailOptions{
		ReportID:           "wr-1",
		WeeklySendDay:      time.Sunday,
		WeeklySendAt:       "09:00",
		DefaultWeeklySendAt: "09:00",
		Recipients: []WeeklyEmailRecipient{
			{UserID: "user-1", EmailEnabled: true, WeeklySendAt: ""},
		},
		Now: func() time.Time { return now },
	})

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.enqueued) != 0 {
		t.Fatalf("expected 0 enqueued jobs at wrong time, got %d", len(producer.enqueued))
	}
}

type fakeProducer struct {
	enqueued []queue.EnqueueRequest
}

func (p *fakeProducer) Enqueue(_ context.Context, req queue.EnqueueRequest) (queue.Job, error) {
	p.enqueued = append(p.enqueued, req)
	return queue.Job{ID: "job-1", Type: req.Type}, nil
}
