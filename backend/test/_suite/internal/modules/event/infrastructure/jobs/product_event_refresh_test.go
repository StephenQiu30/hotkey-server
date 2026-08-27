package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type productEventRefreshTargetReaderFake struct {
	target eventapplication.ProductEventRefreshTargetDTO
}

func (fake productEventRefreshTargetReaderFake) ReadProductEventRefreshScheduleTarget(context.Context,
	eventapplication.ProductEventRefreshScheduleTargetQuery) (eventapplication.ProductEventRefreshTargetDTO, error) {
	return fake.target, nil
}

type productEventRefreshEnqueuerFake struct {
	jobs []queue.Job
	seen map[string]int64
}

func (fake *productEventRefreshEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	fake.jobs = append(fake.jobs, job)
	if fake.seen == nil {
		fake.seen = map[string]int64{}
	}
	if id, found := fake.seen[job.UniqueKey]; found {
		return id, false, nil
	}
	id := int64(len(fake.seen) + 1)
	fake.seen[job.UniqueKey] = id
	return id, true, nil
}

func TestProductEventRefreshSchedulerUsesVersionWindowAndProfilesAsRiverIdentity(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 34, 56, 0, time.UTC)
	targets := productEventRefreshTargetReaderFake{target: eventapplication.ProductEventRefreshTargetDTO{
		MicroEventID: 7, MicroEventVersion: 3, HeatProfileID: 8, HeatProfileVersion: "heat-v2",
		EvidenceStateProfileID: 9, EvidenceStateAlgorithmVersion: eventapplication.CanonicalEvidenceStateAlgorithmVersion}}
	jobs := &productEventRefreshEnqueuerFake{}
	scheduler, err := newProductEventRefreshScheduler(targets, jobs, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	first, err := scheduler.ScheduleProductEventRefresh(context.Background(), eventapplication.ScheduleProductEventRefreshCommand{
		MicroEventID: 7, ExpectedEventVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := scheduler.ScheduleProductEventRefresh(context.Background(), eventapplication.ScheduleProductEventRefreshCommand{
		MicroEventID: 7, ExpectedEventVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Available || !first.Created || second.Created || first.JobID != second.JobID || len(jobs.jobs) != 2 {
		t.Fatalf("schedule replay = first %#v, second %#v, jobs=%#v", first, second, jobs.jobs)
	}
	job := jobs.jobs[0]
	if job.Kind != queue.KindRefreshProductEvent || job.Priority != 7 || job.UniqueKey == "" || len(job.DurableArgs) == 0 {
		t.Fatalf("refresh job = %#v", job)
	}
	var wire map[string]any
	if err := json.Unmarshal(job.DurableArgs, &wire); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"body", "quote", "prompt", "dsn", "credential"} {
		if _, found := wire[forbidden]; found {
			t.Fatalf("refresh args leaked %q: %s", forbidden, job.DurableArgs)
		}
	}
	if got := wire["window_ended_at"]; got != "2026-08-27T12:34:00Z" {
		t.Fatalf("window_ended_at = %#v", got)
	}
}

type productEventRefresherFake struct {
	commands []eventapplication.RefreshProductEventCommand
	err      error
}

func (fake *productEventRefresherFake) Refresh(_ context.Context,
	command eventapplication.RefreshProductEventCommand) (eventapplication.ProductEventRefreshResult, error) {
	fake.commands = append(fake.commands, command)
	if fake.err != nil {
		return eventapplication.ProductEventRefreshResult{}, fake.err
	}
	return eventapplication.ProductEventRefreshResult{Update: eventapplication.ProductEventUpdateDTO{
		ID: 11, Version: 1, MicroEventID: command.MicroEventID, MicroEventVersion: command.ExpectedEventVersion,
		RefreshKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}, nil
}

func TestProductEventRefreshHandlerRejectsUnknownArgsAndDelegatesExactEnvelope(t *testing.T) {
	windowEnd := time.Date(2026, time.August, 27, 12, 34, 0, 0, time.UTC)
	args := productEventRefreshJobArgs{MicroEventID: 7, MicroEventVersion: 3, WindowEndedAt: windowEnd,
		WindowProfile: eventapplication.ProductEventRefreshWindowProfile, HeatProfileVersion: "heat-v2",
		EvidenceStateAlgorithmVersion: eventapplication.CanonicalEvidenceStateAlgorithmVersion}
	encoded, _ := json.Marshal(args)
	job := queue.Job{Kind: queue.KindRefreshProductEvent, UniqueKey: ProductEventRefreshUniqueKey(args),
		DurableArgs: encoded, ScheduledAt: windowEnd, MaxAttempts: 5, Priority: 7}
	fake := &productEventRefresherFake{}
	handler, _ := newProductEventRefreshHandler(fake)
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(fake.commands) != 1 || fake.commands[0].MicroEventID != 7 || fake.commands[0].ExpectedEventVersion != 3 ||
		!fake.commands[0].WindowEndedAt.Equal(windowEnd) {
		t.Fatalf("refresh commands = %#v", fake.commands)
	}
	job.DurableArgs = append(encoded[:len(encoded)-1], []byte(`,"body":"secret"}`)...)
	if err := handler.Handle(context.Background(), job); !queue.IsPermanent(err) {
		t.Fatalf("unknown args error = %v", err)
	}
	fake.err = errors.New("temporary refresh failure")
	job.DurableArgs = encoded
	if err := handler.Handle(context.Background(), job); !queue.IsRetryable(err) {
		t.Fatalf("refresh failure = %v", err)
	}
}
