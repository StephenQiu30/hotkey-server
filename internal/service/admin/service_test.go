package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/service/admin"
)

func TestAuditLogAndFailedJobRetry(t *testing.T) {
	service := admin.NewService(admin.NewMemoryRepository(), admin.Config{})
	ctx := context.Background()

	entry, err := service.RecordAuditLog(ctx, admin.AuditLogInput{
		ActorID:      "usr_admin",
		Action:       "create",
		ResourceType: "source",
		ResourceID:   "src_1",
		Result:       "success",
	})
	if err != nil {
		t.Fatalf("record audit log returned error: %v", err)
	}
	if entry.ID == "" || entry.ActorID != "usr_admin" || entry.ResourceType != "source" {
		t.Fatalf("unexpected audit entry: %#v", entry)
	}

	failedAt := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	failed, err := service.CreateJob(ctx, admin.CreateJobInput{
		Type:           queue.JobTypeCollectSource,
		Payload:        []byte(`{"source_id":"src_1","scheduled_for":"2026-05-31T08:00:00Z"}`),
		Status:         queue.JobStatusFailed,
		Attempt:        3,
		MaxAttempts:    3,
		IdempotencyKey: "collect:src_1:failed",
		LastError:      "upstream timeout",
		NextRunAt:      failedAt,
	})
	if err != nil {
		t.Fatalf("create job returned error: %v", err)
	}

	failedJobs, err := service.ListFailedJobs(ctx)
	if err != nil {
		t.Fatalf("list failed jobs returned error: %v", err)
	}
	if len(failedJobs) != 1 || failedJobs[0].ID != failed.ID {
		t.Fatalf("expected failed job %q, got %#v", failed.ID, failedJobs)
	}

	retried, err := service.RetryJob(ctx, failed.ID)
	if err != nil {
		t.Fatalf("retry job returned error: %v", err)
	}
	if retried.Status != queue.JobStatusPending || retried.Attempt != 0 || retried.LastError != "" {
		t.Fatalf("expected retry to reset failed job, got %#v", retried)
	}
}

func TestConfigStatusDegradesAndRerunsDailyReport(t *testing.T) {
	service := admin.NewService(admin.NewMemoryRepository(), admin.Config{
		PostgreSQLPing: func(context.Context) error { return nil },
		RedisPing:      func(context.Context) error { return errors.New("connection refused") },
		DashScopeKey:   "",
		SMTPHost:       "",
	})
	ctx := context.Background()

	status := service.ConfigStatus(ctx)
	if status.Overall != admin.ComponentStatusDegraded {
		t.Fatalf("expected degraded overall status, got %#v", status)
	}
	if status.Components["redis"].Status != admin.ComponentStatusDegraded {
		t.Fatalf("expected redis degraded, got %#v", status.Components["redis"])
	}
	if status.Components["dashscope"].Reason != "missing_config" {
		t.Fatalf("expected dashscope missing_config, got %#v", status.Components["dashscope"])
	}
	if status.Components["smtp"].Reason != "missing_config" {
		t.Fatalf("expected smtp missing_config, got %#v", status.Components["smtp"])
	}

	job, err := service.RerunDailyReport(ctx, admin.RerunDailyReportInput{
		Date:      "2026-05-31",
		ChannelID: "chn_ai",
		UserID:    "usr_1",
	})
	if err != nil {
		t.Fatalf("rerun daily report returned error: %v", err)
	}
	if job.Type != queue.JobTypeGenerateDailyReport || job.Status != queue.JobStatusPending {
		t.Fatalf("unexpected daily report rerun job: %#v", job)
	}
}
