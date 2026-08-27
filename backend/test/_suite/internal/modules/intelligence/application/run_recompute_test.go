package application

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestAIRunRecomputeServiceSchedulesOnlyOwnedTerminalFailures(t *testing.T) {
	owner := int64(44)
	runs := &aiRunReaderStub{run: domain.Run{ID: 7, Status: domain.RunStatusFailed, OwningJobID: &owner}}
	scheduler := &aiRunSchedulerStub{jobID: 81, created: true}
	service, err := NewAIRunRecomputeService(runs, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Schedule(context.Background(), 7)
	if err != nil || result != (AIRunRecomputeResult{RunID: 7, JobID: 81, Created: true}) || scheduler.runID != 7 {
		t.Fatalf("Schedule() = %#v / %v, scheduled run=%d", result, err, scheduler.runID)
	}

	for _, test := range []struct {
		name string
		run  domain.Run
	}{
		{name: "succeeded", run: domain.Run{ID: 7, Status: domain.RunStatusSucceeded, OwningJobID: &owner}},
		{name: "running", run: domain.Run{ID: 7, Status: domain.RunStatusRunning, OwningJobID: &owner}},
		{name: "unowned", run: domain.Run{ID: 7, Status: domain.RunStatusFailed}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _ := NewAIRunRecomputeService(&aiRunReaderStub{run: test.run}, &aiRunSchedulerStub{})
			if _, err := service.Schedule(context.Background(), 7); !errors.Is(err, sharedrepository.ErrConflict) {
				t.Fatalf("Schedule() error = %v, want conflict", err)
			}
		})
	}
}

type aiRunReaderStub struct {
	run domain.Run
	err error
}

func (stub *aiRunReaderStub) FindRun(context.Context, int64) (domain.Run, error) {
	return stub.run, stub.err
}

type aiRunSchedulerStub struct {
	jobID   int64
	created bool
	runID   int64
	err     error
}

func (stub *aiRunSchedulerStub) ScheduleAIRunRecompute(_ context.Context, runID int64) (int64, bool, error) {
	stub.runID = runID
	return stub.jobID, stub.created, stub.err
}
