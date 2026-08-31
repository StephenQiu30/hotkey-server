package jobs

import (
	"context"
	"fmt"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/scheduler"
)

type XMetricRefreshScheduler struct {
	reader sourcedomain.XMetricRefreshScheduleReader
	jobs   scheduler.Enqueuer
}

func NewXMetricRefreshScheduler(reader sourcedomain.XMetricRefreshScheduleReader, jobs scheduler.Enqueuer) (*XMetricRefreshScheduler, error) {
	if reader == nil || jobs == nil {
		return nil, fmt.Errorf("x metric refresh scheduler dependencies are required")
	}
	return &XMetricRefreshScheduler{reader: reader, jobs: jobs}, nil
}

func (schedulerService *XMetricRefreshScheduler) RunOnce(ctx context.Context, now time.Time) (int, error) {
	if schedulerService == nil || schedulerService.reader == nil || schedulerService.jobs == nil || now.IsZero() {
		return 0, fmt.Errorf("x metric refresh scheduler is not initialized")
	}
	schedules, err := schedulerService.reader.ListXMetricRefreshSchedules(ctx)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, schedule := range schedules {
		if err := schedule.Validate(); err != nil {
			return created, err
		}
		windowStart, windowEnd := xMetricRefreshWindow(now.UTC(), schedule)
		_, wasCreated, err := schedulerService.jobs.Enqueue(ctx, queue.Job{
			Kind:        queue.KindRefreshXMetrics,
			UniqueKey:   scheduler.UniqueKey(queue.KindRefreshXMetrics, schedule.SourceConnectionID, schedule.SourceVersion, windowStart, windowEnd),
			Payload:     queue.Payload{EntityID: schedule.SourceConnectionID, EntityVersion: schedule.SourceVersion, WindowStart: windowStart, WindowEnd: windowEnd},
			ScheduledAt: now.UTC(), MaxAttempts: 1, Priority: 1,
		})
		if err != nil {
			return created, err
		}
		if wasCreated {
			created++
		}
	}
	return created, nil
}

func (schedulerService *XMetricRefreshScheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("x metric refresh scheduler interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := schedulerService.RunOnce(ctx, time.Now().UTC()); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func xMetricRefreshWindow(now time.Time, schedule sourcedomain.XMetricRefreshSchedule) (time.Time, time.Time) {
	effectiveMinutes := schedule.IntervalMinutes
	budgetMinutes := (24*60 + schedule.DailyRequestBudget - 1) / schedule.DailyRequestBudget
	if budgetMinutes > effectiveMinutes {
		effectiveMinutes = budgetMinutes
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	elapsedMinutes := int(now.Sub(dayStart) / time.Minute)
	windowStart := dayStart.Add(time.Duration(elapsedMinutes/effectiveMinutes*effectiveMinutes) * time.Minute)
	windowEnd := windowStart.Add(time.Duration(effectiveMinutes) * time.Minute)
	dayEnd := dayStart.Add(24 * time.Hour)
	if windowEnd.After(dayEnd) {
		windowEnd = dayEnd
	}
	return windowStart, windowEnd
}

type XMetricRefreshUseCase interface {
	Refresh(context.Context, sourceapplication.XMetricRefreshCommand) (sourceapplication.XMetricRefreshResult, error)
}

type XMetricRefreshHandler struct{ service XMetricRefreshUseCase }

func NewXMetricRefreshHandler(service XMetricRefreshUseCase) (*XMetricRefreshHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("x metric refresh handler service is required")
	}
	return &XMetricRefreshHandler{service: service}, nil
}

func (handler *XMetricRefreshHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindRefreshXMetrics); err != nil {
		return queue.NewPermanentError(err)
	}
	_, err := handler.service.Refresh(ctx, sourceapplication.XMetricRefreshCommand{
		SourceConnectionID: job.Payload.EntityID, ExpectedSourceVersion: job.Payload.EntityVersion,
	})
	if err == nil {
		return nil
	}
	if sourcedomain.IsCollectionRetryable(err) {
		return queue.NewRetryableError(err)
	}
	return queue.NewPermanentError(err)
}
