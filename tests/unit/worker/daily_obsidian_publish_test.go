package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	"github.com/StephenQiu30/hotkey-server/internal/worker"
)

func TestMain(m *testing.M) {
	_ = logging.Init("info", "json", "stdout")
	os.Exit(m.Run())
}

type fakeMonitorLister struct {
	monitors []dto.Monitor
}

func (f *fakeMonitorLister) ListActive(ctx context.Context) ([]dto.Monitor, error) {
	return f.monitors, nil
}

type fakeDailyReportService struct {
	reports []dto.Report
}

func (f *fakeDailyReportService) Create(ctx context.Context, userID int64, input dto.CreateInput) (dto.Report, error) {
	item := dto.Report{
		ID:          int64(len(f.reports) + 1),
		UserID:      userID,
		ReportType:  service.TypeDaily,
		PeriodStart: *input.PeriodStart,
		PeriodEnd:   *input.PeriodEnd,
		Subject:     "AI Regulation 日报 2026-07-07",
		Summary:     "今日热点",
		Content:     "## 今日概览\n\n今日热点。\n\n## 热点主题\n\n- AI Regulation",
		Status:      service.StatusDraft,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	f.reports = append(f.reports, item)
	return item, nil
}

type fakeExportRepo struct {
	exports []dto.ReportExport
}

func (f *fakeExportRepo) CreatePending(ctx context.Context, input dto.CreateReportExportInput) (dto.ReportExport, error) {
	item := dto.ReportExport{ID: int64(len(f.exports) + 1), ReportID: input.ReportID, ExportKind: input.ExportKind, TargetPath: input.TargetPath, Status: dto.ExportStatusPending}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkPublished(ctx context.Context, reportID int64, exportKind string, path string, publishedAt time.Time) (dto.ReportExport, error) {
	item := dto.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: dto.ExportStatusPublished, PublishedAt: &publishedAt}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkSkipped(ctx context.Context, reportID int64, exportKind string, path string, skippedAt time.Time) (dto.ReportExport, error) {
	item := dto.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: dto.ExportStatusSkipped, PublishedAt: &skippedAt}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkFailed(ctx context.Context, reportID int64, exportKind string, path string, message string, failedAt time.Time) (dto.ReportExport, error) {
	item := dto.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: dto.ExportStatusFailed, ErrorMessage: message}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) ListByReport(ctx context.Context, reportID int64) ([]dto.ReportExport, error) {
	return f.exports, nil
}

func TestDailyObsidianPublishJobWritesDigestAndDraft(t *testing.T) {
	vault := t.TempDir()
	exports := &fakeExportRepo{}
	job := worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: vault,
		Monitors: &fakeMonitorLister{monitors: []dto.Monitor{{ID: 10, UserID: 7, Name: "AI Regulation", Status: "active"}}},
		Reports:  &fakeDailyReportService{},
		Exports:  exports,
		Now:      func() time.Time { return time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC) },
	})

	targetDate := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	if err := job.RunOnce(context.Background(), targetDate); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	digestPath := filepath.Join(vault, "HotKey", "digests", "daily", "2026-07-07", "ai-regulation.md")
	draftPath := filepath.Join(vault, "HotKey", "publish", "drafts", "2026-07-07", "ai-regulation.md")
	for _, path := range []string{digestPath, draftPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %s: %v", path, err)
		}
	}
	if len(exports.exports) != 4 {
		t.Fatalf("expected exactly 4 export records (2 pending + 2 published), got %d", len(exports.exports))
	}
}

func TestDailyObsidianPublishJobSkipsExistingFiles(t *testing.T) {
	vault := t.TempDir()
	digestPath := filepath.Join(vault, "HotKey", "digests", "daily", "2026-07-07", "ai-regulation.md")
	if err := os.MkdirAll(filepath.Dir(digestPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(digestPath, []byte("edited draft"), 0o644); err != nil {
		t.Fatalf("seed digest: %v", err)
	}

	exports := &fakeExportRepo{}
	job := worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: vault,
		Monitors: &fakeMonitorLister{monitors: []dto.Monitor{{ID: 10, UserID: 7, Name: "AI Regulation", Status: "active"}}},
		Reports:  &fakeDailyReportService{},
		Exports:  exports,
		Now:      func() time.Time { return time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC) },
	})
	targetDate := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	if err := job.RunOnce(context.Background(), targetDate); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	got, err := os.ReadFile(digestPath)
	if err != nil {
		t.Fatalf("read digest: %v", err)
	}
	if string(got) != "edited draft" {
		t.Fatalf("existing digest overwritten: %q", string(got))
	}
	foundSkipped := false
	for _, item := range exports.exports {
		if item.ExportKind == dto.ExportKindDailyDigest && item.Status == dto.ExportStatusSkipped {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Fatalf("expected skipped export record, got %+v", exports.exports)
	}
}

type fakeRunRepo struct {
	started map[string]bool
}

func (f *fakeRunRepo) TryStart(ctx context.Context, runKey string, runType string, targetDate time.Time, startedAt time.Time) (bool, error) {
	if f.started == nil {
		f.started = map[string]bool{}
	}
	if f.started[runKey] {
		return false, nil
	}
	f.started[runKey] = true
	return true, nil
}
func (f *fakeRunRepo) MarkFinished(ctx context.Context, runKey string, finishedAt time.Time) error { return nil }
func (f *fakeRunRepo) MarkFailed(ctx context.Context, runKey string, message string, failedAt time.Time) error { return nil }

func TestDailyObsidianPublishJobSkipsDuplicateRunKey(t *testing.T) {
	vault := t.TempDir()
	reportSvc := &fakeDailyReportService{}
	runs := &fakeRunRepo{}
	job := worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: vault,
		Monitors: &fakeMonitorLister{monitors: []dto.Monitor{{ID: 10, UserID: 7, Name: "AI Regulation", Status: "active"}}},
		Reports:  reportSvc,
		Exports:  &fakeExportRepo{},
		Runs:     runs,
		Now:      func() time.Time { return time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC) },
	})
	targetDate := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	if err := job.RunOnce(context.Background(), targetDate); err != nil {
		t.Fatalf("first RunOnce returned error: %v", err)
	}
	if err := job.RunOnce(context.Background(), targetDate); err != nil {
		t.Fatalf("second RunOnce returned error: %v", err)
	}
	if len(reportSvc.reports) != 1 {
		t.Fatalf("reports generated = %d, want 1", len(reportSvc.reports))
	}
}

func TestDailyObsidianPublishJobMissingVaultRoot(t *testing.T) {
	job := worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: "",
	})
	err := job.RunOnce(context.Background(), time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC))
	if err != service.ErrMissingVaultRoot {
		t.Fatalf("error = %v, want %v", err, service.ErrMissingVaultRoot)
	}
}
