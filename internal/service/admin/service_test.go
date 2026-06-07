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

func TestDeleteAccountReturnsCleanupTask(t *testing.T) {
	repo := admin.NewMemoryRepository()
	repo.SetUser("usr_1", admin.UserRecord{
		ID:           "usr_1",
		Email:        "alice@example.com",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ12",
		Role:         "user",
		Status:       "active",
	})
	repo.SetRSSFeed("usr_1", admin.RSSFeedRecord{UserID: "usr_1", TokenHash: "hash_abc", Enabled: true})
	repo.AddDailyReport(admin.DailyReportRecord{ID: "rpt_1", UserID: "usr_1", Date: "2026-06-01"})
	repo.AddDailyReport(admin.DailyReportRecord{ID: "rpt_2", UserID: "usr_2", Date: "2026-06-01"})

	svc := admin.NewService(repo, admin.Config{})
	ctx := context.Background()

	task, err := svc.DeleteAccount(ctx, "usr_1")
	if err != nil {
		t.Fatalf("delete account returned error: %v", err)
	}
	if task.UserID != "usr_1" {
		t.Fatalf("expected task user_id usr_1, got %s", task.UserID)
	}
	if task.Status != admin.CleanupStatusCompleted {
		t.Fatalf("expected completed cleanup status, got %s", task.Status)
	}

	if repo.HasUser("usr_1") {
		t.Fatalf("expected user usr_1 to be deleted")
	}
	if repo.RSSFeedCount() != 0 {
		t.Fatalf("expected rss feeds cleaned up, got %d", repo.RSSFeedCount())
	}
	reports := repo.DailyReports()
	if len(reports) != 1 || reports[0].UserID != "usr_2" {
		t.Fatalf("expected only usr_2 daily reports remaining, got %#v", reports)
	}
}

func TestDeleteAccountRejectsInvalidInput(t *testing.T) {
	svc := admin.NewService(admin.NewMemoryRepository(), admin.Config{})
	ctx := context.Background()

	if _, err := svc.DeleteAccount(ctx, "   "); !errors.Is(err, admin.ErrInvalidInput) {
		t.Fatalf("expected invalid input for blank user id, got %v", err)
	}
	if _, err := svc.DeleteAccount(ctx, "nonexistent"); !errors.Is(err, admin.ErrNotFound) {
		t.Fatalf("expected not found for nonexistent user, got %v", err)
	}
}

func TestDeleteAccountPartialFailureIsRetryable(t *testing.T) {
	repo := admin.NewMemoryRepository()
	repo.SetUser("usr_1", admin.UserRecord{
		ID:           "usr_1",
		Email:        "bob@example.com",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ12",
		Role:         "user",
		Status:       "active",
	})
	repo.SetDeleteReportError(errors.New("database timeout"))

	svc := admin.NewService(repo, admin.Config{})
	ctx := context.Background()

	task, err := svc.DeleteAccount(ctx, "usr_1")
	if err != nil {
		t.Fatalf("delete account returned error: %v", err)
	}
	if task.Status != admin.CleanupStatusFailed {
		t.Fatalf("expected failed cleanup status, got %s", task.Status)
	}
	failedStep := findStep(task.Steps, "delete_daily_reports")
	if failedStep == nil || failedStep.Status != admin.CleanupStatusFailed {
		t.Fatalf("expected delete_daily_reports step to be failed, got %#v", task.Steps)
	}

	repo.SetDeleteReportError(nil)
	retried, err := svc.RetryCleanup(ctx, task.ID)
	if err != nil {
		t.Fatalf("retry cleanup returned error: %v", err)
	}
	if retried.Status != admin.CleanupStatusCompleted {
		t.Fatalf("expected completed after retry, got %s", retried.Status)
	}
}

func TestAuditLogSanitizesSensitiveFields(t *testing.T) {
	svc := admin.NewService(admin.NewMemoryRepository(), admin.Config{})
	ctx := context.Background()

	entry, err := svc.RecordAuditLog(ctx, admin.AuditLogInput{
		ActorID:      "usr_1",
		Action:       "create",
		ResourceType: "source",
		ResourceID:   "src_1",
		Result:       "success",
		Metadata: map[string]string{
			"token":         "secret_access_token_abc123",
			"password_hash": "$2a$10$hashvalue",
			"api_key":       "sk-1234567890",
			"name":          "OpenAI Blog",
		},
	})
	if err != nil {
		t.Fatalf("record audit log returned error: %v", err)
	}
	if entry.Metadata["token"] != "[REDACTED]" {
		t.Fatalf("expected token to be redacted, got %s", entry.Metadata["token"])
	}
	if entry.Metadata["password_hash"] != "[REDACTED]" {
		t.Fatalf("expected password_hash to be redacted, got %s", entry.Metadata["password_hash"])
	}
	if entry.Metadata["api_key"] != "[REDACTED]" {
		t.Fatalf("expected api_key to be redacted, got %s", entry.Metadata["api_key"])
	}
	if entry.Metadata["name"] != "OpenAI Blog" {
		t.Fatalf("expected name to be preserved, got %s", entry.Metadata["name"])
	}
}

func TestCleanupStatusLookup(t *testing.T) {
	repo := admin.NewMemoryRepository()
	repo.SetUser("usr_1", admin.UserRecord{
		ID:     "usr_1",
		Email:  "carol@example.com",
		Status: "active",
	})
	svc := admin.NewService(repo, admin.Config{})
	ctx := context.Background()

	task, err := svc.DeleteAccount(ctx, "usr_1")
	if err != nil {
		t.Fatalf("delete account: %v", err)
	}

	found, err := svc.CleanupStatus(ctx, task.ID)
	if err != nil {
		t.Fatalf("cleanup status: %v", err)
	}
	if found.ID != task.ID || found.Status != task.Status {
		t.Fatalf("expected matching cleanup task, got %#v", found)
	}

	if _, err := svc.CleanupStatus(ctx, "nonexistent"); !errors.Is(err, admin.ErrNotFound) {
		t.Fatalf("expected not found for nonexistent task, got %v", err)
	}
}

func findStep(steps []admin.CleanupStep, name string) *admin.CleanupStep {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}
	}
	return nil
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
