package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

// 测试：DailyReportScheduler 在每天指定时间入队 generate_daily_report
func TestDailyReportSchedulerEnqueuesGenerateReportJob(t *testing.T) {
	ctx := context.Background()
	producer := &recordingProducer{}
	now := time.Date(2026, 6, 8, 6, 0, 0, 0, time.UTC)
	scheduler := NewDailyReportScheduler(producer, DailyReportOptions{
		ReportAt: "06:00",
		Now:      func() time.Time { return now },
	})

	if err := scheduler.Tick(ctx); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 1 {
		t.Fatalf("expected 1 enqueue request, got %d", len(producer.requests))
	}

	req := producer.requests[0]
	if req.Type != queue.JobTypeGenerateDailyReport {
		t.Fatalf("expected generate_daily_report job, got %s", req.Type)
	}

	var payload queue.GenerateDailyReportPayload
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		t.Fatalf("payload was not generate_daily_report payload: %v", err)
	}
	if payload.Date != "2026-06-08" {
		t.Fatalf("expected date 2026-06-08, got %s", payload.Date)
	}

	expectedKey := "generate_daily_report:2026-06-08"
	if req.IdempotencyKey != expectedKey {
		t.Fatalf("unexpected idempotency key %q, expected %q", req.IdempotencyKey, expectedKey)
	}
}

// 测试：非指定时间不入队
func TestDailyReportSchedulerSkipsWhenNotReportTime(t *testing.T) {
	ctx := context.Background()
	producer := &recordingProducer{}
	now := time.Date(2026, 6, 8, 7, 30, 0, 0, time.UTC)
	scheduler := NewDailyReportScheduler(producer, DailyReportOptions{
		ReportAt: "06:00",
		Now:      func() time.Time { return now },
	})

	if err := scheduler.Tick(ctx); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 0 {
		t.Fatalf("expected 0 enqueue requests at wrong time, got %d", len(producer.requests))
	}
}

// 测试：nil producer panic
func TestDailyReportSchedulerRejectsNilProducer(t *testing.T) {
	assertPanic(t, func() {
		NewDailyReportScheduler(nil, DailyReportOptions{ReportAt: "06:00"})
	})
}

// 测试：默认时间 06:00
func TestDailyReportSchedulerDefaultTime(t *testing.T) {
	ctx := context.Background()
	producer := &recordingProducer{}
	now := time.Date(2026, 6, 8, 6, 0, 0, 0, time.UTC)
	scheduler := NewDailyReportScheduler(producer, DailyReportOptions{
		Now: func() time.Time { return now },
	})

	if err := scheduler.Tick(ctx); err != nil {
		t.Fatalf("tick failed: %v", err)
	}
	if len(producer.requests) != 1 {
		t.Fatalf("expected 1 enqueue request with default time, got %d", len(producer.requests))
	}
}
