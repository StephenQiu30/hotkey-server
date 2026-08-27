package application

import (
	"context"
	"testing"
	"time"
)

type contentMicroEventReaderFake struct {
	microEventIDs []int64
}

func (fake contentMicroEventReaderFake) ListMetricMicroEventIDsForContent(context.Context, int64) ([]int64, error) {
	return append([]int64(nil), fake.microEventIDs...), nil
}

type productEventRefreshSchedulerFake struct {
	commands []ScheduleProductEventRefreshCommand
}

func (fake *productEventRefreshSchedulerFake) ScheduleProductEventRefresh(_ context.Context, command ScheduleProductEventRefreshCommand) (ScheduleProductEventRefreshResult, error) {
	fake.commands = append(fake.commands, command)
	return ScheduleProductEventRefreshResult{MicroEventID: command.MicroEventID, MicroEventVersion: 1,
		JobID: int64(len(fake.commands)), Created: true, Available: true}, nil
}

func TestContentMetricRefreshSchedulesOnlyProductEventRiverJobs(t *testing.T) {
	now := time.Date(2026, time.August, 12, 7, 43, 29, 0, time.UTC)
	scheduler := &productEventRefreshSchedulerFake{}
	service, err := NewContentMetricRefreshServiceWithClock(
		contentMicroEventReaderFake{microEventIDs: []int64{3, 7}}, scheduler,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewContentMetricRefreshServiceWithClock: %v", err)
	}
	if err := service.RecomputeMetricsForContent(context.Background(), 11); err != nil {
		t.Fatalf("RecomputeMetricsForContent: %v", err)
	}
	want := []ScheduleProductEventRefreshCommand{
		{MicroEventID: 3, WindowEndedAt: now.Truncate(time.Minute)},
		{MicroEventID: 7, WindowEndedAt: now.Truncate(time.Minute)},
	}
	if len(scheduler.commands) != len(want) {
		t.Fatalf("refresh commands = %#v, want %#v", scheduler.commands, want)
	}
	for index := range want {
		if scheduler.commands[index] != want[index] {
			t.Fatalf("refresh command[%d] = %#v, want %#v", index, scheduler.commands[index], want[index])
		}
	}
}
