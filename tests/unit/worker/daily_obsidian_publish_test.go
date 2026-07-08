package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/StephenQiu30/hotkey-server/internal/worker"
)

type fakeMonitorLister struct {
	monitors []monitor.Monitor
}

func (f *fakeMonitorLister) ListActive(ctx context.Context) ([]monitor.Monitor, error) {
	return f.monitors, nil
}

type fakeDailyReportService struct {
	reports []report.Report
}

func (f *fakeDailyReportService) Create(ctx context.Context, userID int64, input report.CreateInput) (report.Report, error) {
	item := report.Report{
		ID:          int64(len(f.reports) + 1),
		UserID:      userID,
		ReportType:  report.TypeDaily,
		PeriodStart: *input.PeriodStart,
		PeriodEnd:   *input.PeriodEnd,
		Subject:     "AI Regulation 日报 2026-07-07",
		Summary:     "今日热点",
		Content:     "## 今日概览\n\n今日热点。\n\n## 热点主题\n\n- AI Regulation",
		Status:      report.StatusDraft,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	f.reports = append(f.reports, item)
	return item, nil
}

type fakeExportRepo struct {
	exports []report.ReportExport
}

func (f *fakeExportRepo) CreatePending(ctx context.Context, input report.CreateReportExportInput) (report.ReportExport, error) {
	item := report.ReportExport{ID: int64(len(f.exports) + 1), ReportID: input.ReportID, ExportKind: input.ExportKind, TargetPath: input.TargetPath, Status: report.ExportStatusPending}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkPublished(ctx context.Context, reportID int64, exportKind string, path string, publishedAt time.Time) (report.ReportExport, error) {
	item := report.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: report.ExportStatusPublished, PublishedAt: &publishedAt}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkSkipped(ctx context.Context, reportID int64, exportKind string, path string, skippedAt time.Time) (report.ReportExport, error) {
	item := report.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: report.ExportStatusSkipped, PublishedAt: &skippedAt}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) MarkFailed(ctx context.Context, reportID int64, exportKind string, path string, message string, failedAt time.Time) (report.ReportExport, error) {
	item := report.ReportExport{ID: int64(len(f.exports) + 1), ReportID: reportID, ExportKind: exportKind, TargetPath: path, Status: report.ExportStatusFailed, ErrorMessage: message}
	f.exports = append(f.exports, item)
	return item, nil
}
func (f *fakeExportRepo) ListByReport(ctx context.Context, reportID int64) ([]report.ReportExport, error) {
	return f.exports, nil
}

func TestDailyObsidianPublishJobWritesDigestAndDraft(t *testing.T) {
	vault := t.TempDir()
	exports := &fakeExportRepo{}
	job := worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: vault,
		Monitors: &fakeMonitorLister{monitors: []monitor.Monitor{{ID: 10, UserID: 7, Name: "AI Regulation", Status: "active"}}},
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
	if len(exports.exports) < 4 {
		t.Fatalf("expected pending and final export records, got %d", len(exports.exports))
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
		Monitors: &fakeMonitorLister{monitors: []monitor.Monitor{{ID: 10, UserID: 7, Name: "AI Regulation", Status: "active"}}},
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
		if item.ExportKind == report.ExportKindDailyDigest && item.Status == report.ExportStatusSkipped {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Fatalf("expected skipped export record, got %+v", exports.exports)
	}
}
