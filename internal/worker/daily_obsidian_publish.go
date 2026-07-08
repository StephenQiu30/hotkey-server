package worker

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
	"github.com/StephenQiu30/hotkey-server/internal/report"
)

type MonitorLister interface {
	ListActive(ctx context.Context) ([]monitor.Monitor, error)
}

type DailyReportService interface {
	Create(ctx context.Context, userID int64, input report.CreateInput) (report.Report, error)
}

type DailyObsidianPublishDeps struct {
	VaultRoot string
	Monitors  MonitorLister
	Reports   DailyReportService
	Exports   report.ExportRepository
	Runs      RunRepository
	Now       func() time.Time
}

type DailyObsidianPublishJob struct {
	deps DailyObsidianPublishDeps
}

func NewDailyObsidianPublishJob(deps DailyObsidianPublishDeps) *DailyObsidianPublishJob {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &DailyObsidianPublishJob{deps: deps}
}

func (j *DailyObsidianPublishJob) RunOnce(ctx context.Context, targetDate time.Time) (runErr error) {
	if j.deps.VaultRoot == "" {
		return obsidian.ErrMissingVaultRoot
	}
	runKey := RunKeyForDate(targetDate)
	if j.deps.Runs != nil {
		started, err := j.deps.Runs.TryStart(ctx, runKey, "daily-obsidian-publish", targetDate, j.deps.Now())
		if err != nil {
			return err
		}
		if !started {
			return nil
		}
		defer func() {
			if runErr != nil {
				_ = j.deps.Runs.MarkFailed(context.Background(), runKey, runErr.Error(), j.deps.Now())
				return
			}
			_ = j.deps.Runs.MarkFinished(context.Background(), runKey, j.deps.Now())
		}()
	}
	monitors, err := j.deps.Monitors.ListActive(ctx)
	if err != nil {
		return err
	}
	for _, m := range monitors {
		periodStart := targetDate
		periodEnd := targetDate
		item, err := j.deps.Reports.Create(ctx, m.UserID, report.CreateInput{
			ReportType:  report.TypeDaily,
			PeriodStart: &periodStart,
			PeriodEnd:   &periodEnd,
			MonitorID:   m.ID,
		})
		if err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		runErr = errors.Join(runErr, j.exportOne(ctx, item, m, obsidian.ExportDailyDigest, targetDate))
		runErr = errors.Join(runErr, j.exportOne(ctx, item, m, obsidian.ExportPublishDraft, targetDate))
	}
	return runErr
}

func (j *DailyObsidianPublishJob) exportOne(ctx context.Context, item report.Report, m monitor.Monitor, kind obsidian.ExportKind, targetDate time.Time) error {
	path, err := obsidian.BuildPath(j.deps.VaultRoot, obsidian.PathInput{
		Kind:        kind,
		Date:        targetDate,
		MonitorName: m.Name,
	})
	if err != nil {
		_, _ = j.deps.Exports.MarkFailed(ctx, item.ID, string(kind), "", err.Error(), j.deps.Now())
		return err
	}
	if _, err := j.deps.Exports.CreatePending(ctx, report.CreateReportExportInput{
		ReportID:   item.ID,
		ExportKind: string(kind),
		TargetPath: path,
	}); err != nil {
		return err
	}
	markdown, err := obsidian.RenderMarkdown(obsidian.MarkdownInput{
		Kind:        kind,
		Date:        targetDate,
		ReportID:    item.ID,
		MonitorID:   m.ID,
		MonitorName: m.Name,
		Title:       item.Subject,
		Content:     item.Content,
	})
	if err != nil {
		_, _ = j.deps.Exports.MarkFailed(ctx, item.ID, string(kind), path, err.Error(), j.deps.Now())
		return err
	}
	result, err := obsidian.WriteAtomicNoOverwrite(path, []byte(markdown))
	if err != nil {
		_, _ = j.deps.Exports.MarkFailed(ctx, item.ID, string(kind), path, err.Error(), j.deps.Now())
		return err
	}
	if result.Skipped {
		_, err = j.deps.Exports.MarkSkipped(ctx, item.ID, string(kind), path, j.deps.Now())
		return err
	}
	_, err = j.deps.Exports.MarkPublished(ctx, item.ID, string(kind), path, j.deps.Now())
	return err
}
