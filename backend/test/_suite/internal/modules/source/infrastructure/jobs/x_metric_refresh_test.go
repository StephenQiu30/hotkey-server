package jobs

import (
	"context"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestXMetricRefreshSchedulerUsesStableDailyBudgetedSlots(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 12, 13, 17, 0, 0, time.UTC)
	reader := &xMetricScheduleReaderFake{items: []sourcedomain.XMetricRefreshSchedule{{
		SourceConnectionID: 7, SourceVersion: 3, IntervalMinutes: 15, DailyRequestBudget: 2,
	}}}
	store := &xMetricEnqueuerFake{}
	scheduler, err := NewXMetricRefreshScheduler(reader, store)
	if err != nil {
		t.Fatal(err)
	}
	created, err := scheduler.RunOnce(context.Background(), now)
	if err != nil || created != 1 || len(store.jobs) != 1 {
		t.Fatalf("RunOnce = %d, %v, jobs %#v", created, err, store.jobs)
	}
	job := store.jobs[0]
	if job.Kind != queue.KindRefreshXMetrics || job.Payload.EntityID != 7 || job.Payload.EntityVersion != 3 ||
		job.MaxAttempts != 1 || !job.Payload.WindowStart.Equal(time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)) ||
		!job.Payload.WindowEnd.Equal(time.Date(2026, time.August, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("scheduled job = %#v", job)
	}
	firstKey := job.UniqueKey
	store.created = false
	created, err = scheduler.RunOnce(context.Background(), now.Add(time.Minute))
	if err != nil || created != 0 || len(store.jobs) != 2 || store.jobs[1].UniqueKey != firstKey {
		t.Fatalf("repeated RunOnce = %d, %v, jobs %#v", created, err, store.jobs)
	}
}

func TestXMetricRefreshHandlerUsesOnlySourceIdentityEnvelope(t *testing.T) {
	t.Parallel()
	service := &xMetricRefreshUseCaseFake{}
	handler, err := NewXMetricRefreshHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	job := queue.Job{
		Kind: queue.KindRefreshXMetrics, UniqueKey: "refresh-7", Payload: queue.Payload{EntityID: 7, EntityVersion: 3},
		ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1,
	}
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if service.command != (sourceapplication.XMetricRefreshCommand{SourceConnectionID: 7, ExpectedSourceVersion: 3}) {
		t.Fatalf("refresh command = %#v", service.command)
	}
}

type xMetricScheduleReaderFake struct {
	items []sourcedomain.XMetricRefreshSchedule
}

func (fake *xMetricScheduleReaderFake) ListXMetricRefreshSchedules(context.Context) ([]sourcedomain.XMetricRefreshSchedule, error) {
	return fake.items, nil
}

type xMetricEnqueuerFake struct {
	jobs    []queue.Job
	created bool
}

func (fake *xMetricEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	fake.jobs = append(fake.jobs, job)
	created := fake.created
	if len(fake.jobs) == 1 {
		created = true
	}
	return 1, created, nil
}

type xMetricRefreshUseCaseFake struct {
	command sourceapplication.XMetricRefreshCommand
}

func (fake *xMetricRefreshUseCaseFake) Refresh(_ context.Context, command sourceapplication.XMetricRefreshCommand) (sourceapplication.XMetricRefreshResult, error) {
	fake.command = command
	return sourceapplication.XMetricRefreshResult{}, nil
}
