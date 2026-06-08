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

func TestDeleteAccountCleansUpSubresources(t *testing.T) {
	repo := admin.NewMemoryRepository()
	repo.SetUser("usr_del", admin.UserRecord{ID: "usr_del", Email: "del@example.com"})
	repo.SetRSSFeed("usr_del", admin.RSSFeedRecord{UserID: "usr_del", Token: "rss_tok"})
	repo.SetDailyReport("rpt_1", admin.DailyReportRecord{ID: "rpt_1", UserID: "usr_del"})

	service := admin.NewService(repo, admin.Config{})
	ctx := context.Background()

	task, err := service.DeleteAccount(ctx, "usr_del")
	if err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if task.Status != admin.CleanupStatusCompleted {
		t.Fatalf("expected completed cleanup, got %s", task.Status)
	}
	if len(task.Steps) != 3 {
		t.Fatalf("expected 3 cleanup steps, got %d", len(task.Steps))
	}
	for _, step := range task.Steps {
		if step.Status != admin.CleanupStatusCompleted {
			t.Fatalf("expected step %s completed, got %s", step.Name, step.Status)
		}
	}

	if _, err := repo.UserByID(ctx, "usr_del"); !errors.Is(err, admin.ErrNotFound) {
		t.Fatalf("expected user deleted, got %v", err)
	}
}

func TestDeleteAccountInvalidInput(t *testing.T) {
	service := admin.NewService(admin.NewMemoryRepository(), admin.Config{})
	ctx := context.Background()

	if _, err := service.DeleteAccount(ctx, "   "); !errors.Is(err, admin.ErrInvalidInput) {
		t.Fatalf("expected invalid input for blank userID, got %v", err)
	}
	if _, err := service.DeleteAccount(ctx, "nonexistent"); !errors.Is(err, admin.ErrNotFound) {
		t.Fatalf("expected not found for nonexistent user, got %v", err)
	}
}

func TestDeleteAccountPartialFailureAndRetry(t *testing.T) {
	repo := admin.NewMemoryRepository()
	repo.SetUser("usr_pf", admin.UserRecord{ID: "usr_pf", Email: "pf@example.com"})
	repo.SetDeleteReportError(errors.New("db connection lost"))

	service := admin.NewService(repo, admin.Config{})
	ctx := context.Background()

	task, err := service.DeleteAccount(ctx, "usr_pf")
	if err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if task.Status != admin.CleanupStatusFailed {
		t.Fatalf("expected failed cleanup, got %s", task.Status)
	}

	// First step should have failed, remaining steps should be pending
	if task.Steps[0].Status != admin.CleanupStatusFailed {
		t.Fatalf("expected first step failed, got %s", task.Steps[0].Status)
	}
	for i := 1; i < len(task.Steps); i++ {
		if task.Steps[i].Status != admin.CleanupStatusPending {
			t.Fatalf("expected step %d pending (not yet executed), got %s", i, task.Steps[i].Status)
		}
	}

	// Fix the error and retry
	repo.SetDeleteReportError(nil)
	retried, err := service.RetryCleanup(ctx, task.ID)
	if err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if retried.Status != admin.CleanupStatusCompleted {
		t.Fatalf("expected completed after retry, got %s", retried.Status)
	}
	for _, step := range retried.Steps {
		if step.Status != admin.CleanupStatusCompleted {
			t.Fatalf("expected step %s completed after retry, got %s", step.Name, step.Status)
		}
	}
}

func TestAuditMetadataRedactsSensitiveFields(t *testing.T) {
	service := admin.NewService(admin.NewMemoryRepository(), admin.Config{})
	ctx := context.Background()

	entry, err := service.RecordAuditLog(ctx, admin.AuditLogInput{
		ActorID:      "usr_admin",
		Action:       "connect",
		ResourceType: "authorization",
		ResourceID:   "az_1",
		Result:       "success",
		Metadata: map[string]string{
			"access_token": "secret_token_123",
			"platform":     "x",
			"api_key":      "key_abc",
			"password":     "hunter2",
		},
	})
	if err != nil {
		t.Fatalf("record audit log: %v", err)
	}
	if entry.Metadata["access_token"] != "[REDACTED]" {
		t.Fatalf("expected access_token redacted, got %q", entry.Metadata["access_token"])
	}
	if entry.Metadata["api_key"] != "[REDACTED]" {
		t.Fatalf("expected api_key redacted, got %q", entry.Metadata["api_key"])
	}
	if entry.Metadata["password"] != "[REDACTED]" {
		t.Fatalf("expected password redacted, got %q", entry.Metadata["password"])
	}
	if entry.Metadata["platform"] != "x" {
		t.Fatalf("expected platform preserved, got %q", entry.Metadata["platform"])
	}
}

func TestCleanupTaskStepsDefensiveCopy(t *testing.T) {
	repo := admin.NewMemoryRepository()
	repo.SetUser("usr_dc", admin.UserRecord{ID: "usr_dc", Email: "dc@example.com"})

	service := admin.NewService(repo, admin.Config{})
	ctx := context.Background()

	task, err := service.DeleteAccount(ctx, "usr_dc")
	if err != nil {
		t.Fatalf("delete account: %v", err)
	}

	// Mutate the returned steps slice
	originalLen := len(task.Steps)
	task.Steps[0].Status = "mutated"
	task.Steps = append(task.Steps, admin.CleanupStep{Name: "extra"})

	// Fetch the task again — internal state should be unaffected
	fetched, err := service.CleanupStatus(ctx, task.ID)
	if err != nil {
		t.Fatalf("cleanup status: %v", err)
	}
	if len(fetched.Steps) != originalLen {
		t.Fatalf("expected %d steps in stored task, got %d", originalLen, len(fetched.Steps))
	}
	if fetched.Steps[0].Status == "mutated" {
		t.Fatal("expected stored task steps to be immune to external mutation")
	}
}

func TestCleanupStatusLookup(t *testing.T) {
	repo := admin.NewMemoryRepository()
	repo.SetUser("usr_cl", admin.UserRecord{ID: "usr_cl", Email: "cl@example.com"})

	service := admin.NewService(repo, admin.Config{})
	ctx := context.Background()

	task, err := service.DeleteAccount(ctx, "usr_cl")
	if err != nil {
		t.Fatalf("delete account: %v", err)
	}

	found, err := service.CleanupStatus(ctx, task.ID)
	if err != nil {
		t.Fatalf("cleanup status: %v", err)
	}
	if found.ID != task.ID {
		t.Fatalf("expected task ID %s, got %s", task.ID, found.ID)
	}
	if found.Status != admin.CleanupStatusCompleted {
		t.Fatalf("expected completed, got %s", found.Status)
	}

	if _, err := service.CleanupStatus(ctx, "nonexistent"); !errors.Is(err, admin.ErrNotFound) {
		t.Fatalf("expected not found for nonexistent task, got %v", err)
	}
}
