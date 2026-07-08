package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"go.uber.org/zap"
)

type MonitorLister interface {
	ListActive(ctx context.Context) ([]dto.Monitor, error)
}

type DailyReportService interface {
	Create(ctx context.Context, userID int64, input dto.CreateInput) (dto.Report, error)
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

func (j *DailyObsidianPublishJob) Type() string { return "digest.run" }

func (j *DailyObsidianPublishJob) Handle(ctx context.Context, msg queue.Message) error {
	var payload struct {
		TargetDate string `json:"target_date"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	targetDate, err := time.Parse("2006-01-02", payload.TargetDate)
	if err != nil {
		return err
	}
	return j.RunOnce(ctx, targetDate)
}

func (j *DailyObsidianPublishJob) DedupeEnabled() bool { return false }

func RunKeyForDate(date time.Time) string {
	return "daily-obsidian-publish:" + date.Format("2006-01-02")
}

func ResolveTargetDate(now time.Time, cfg struct {
	Timezone string
	Target   string
}) (time.Time, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(loc)
	switch cfg.Target {
	case "", "yesterday":
		local = local.AddDate(0, 0, -1)
	case "today":
	default:
		return time.Time{}, fmt.Errorf("invalid daily digest target: %s", cfg.Target)
	}
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc), nil
}

func (j *DailyObsidianPublishJob) RunOnce(ctx context.Context, targetDate time.Time) (runErr error) {
	if j.deps.VaultRoot == "" {
		return obsidian.ErrMissingVaultRoot
	}
	runKey := RunKeyForDate(targetDate)
	log := logging.L().With(
		zap.String("run_key", runKey),
		zap.String("target_date", targetDate.Format("2006-01-02")),
	)
	log.Info("starting daily obsidian publish run")
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
		item, err := j.deps.Reports.Create(ctx, m.UserID, dto.CreateInput{
			ReportType:  report.TypeDaily,
			PeriodStart: &periodStart,
			PeriodEnd:   &periodEnd,
			MonitorID:   m.ID,
		})
		if err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		runErr = errors.Join(runErr, j.exportOne(ctx, item, m, dto.ExportDailyDigest, targetDate))
		runErr = errors.Join(runErr, j.exportOne(ctx, item, m, dto.ExportPublishDraft, targetDate))
	}
	if runErr != nil {
		log.Error("daily obsidian publish run finished with errors",
			zap.Error(runErr),
		)
	} else {
		log.Info("daily obsidian publish run completed successfully")
	}
	return runErr
}

func (j *DailyObsidianPublishJob) exportOne(ctx context.Context, item dto.Report, m dto.Monitor, kind dto.ExportKind, targetDate time.Time) error {
	log := logging.L().With(
		zap.Int64("report_id", item.ID),
		zap.String("export_kind", string(kind)),
		zap.String("target_date", targetDate.Format("2006-01-02")),
	)
	path, err := obsidian.BuildPath(j.deps.VaultRoot, dto.PathInput{
		Kind:        kind,
		Date:        targetDate,
		MonitorName: m.Name,
	})
	if err != nil {
		_, _ = j.deps.Exports.MarkFailed(ctx, item.ID, string(kind), "", err.Error(), j.deps.Now())
		log.Error("export failed: build path", zap.Error(err))
		return err
	}
	if _, err := j.deps.Exports.CreatePending(ctx, report.CreateReportExportInput{
		ReportID:   item.ID,
		ExportKind: string(kind),
		TargetPath: path,
	}); err != nil {
		log.Error("export failed: create pending", zap.Error(err))
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
		log.Error("export failed: render markdown", zap.Error(err))
		return err
	}
	result, err := obsidian.WriteAtomicNoOverwrite(path, []byte(markdown))
	if err != nil {
		_, _ = j.deps.Exports.MarkFailed(ctx, item.ID, string(kind), path, err.Error(), j.deps.Now())
		log.Error("export failed: write atomic", zap.Error(err))
		return err
	}
	if result.Skipped {
		if _, markErr := j.deps.Exports.MarkSkipped(ctx, item.ID, string(kind), path, j.deps.Now()); markErr != nil {
			log.Error("export mark skipped failed", zap.Error(markErr))
			return markErr
		}
		log.Info("export skipped: file already exists")
		return nil
	}
	_, err = j.deps.Exports.MarkPublished(ctx, item.ID, string(kind), path, j.deps.Now())
	if err != nil {
		log.Error("export failed: mark published", zap.Error(err))
	}
	return err
}
