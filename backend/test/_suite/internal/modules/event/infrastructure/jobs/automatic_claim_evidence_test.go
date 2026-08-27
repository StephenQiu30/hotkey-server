package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestAutomaticClaimEvidenceSchedulerPersistsOnlyEventAndDocumentIdentity(t *testing.T) {
	now := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	enqueuer := &automaticClaimEvidenceEnqueuerFake{id: 101, created: true}
	scheduler, err := newAutomaticClaimEvidenceScheduler(enqueuer, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := scheduler.ScheduleAutomaticClaimEvidence(context.Background(), eventapplication.ScheduleAutomaticClaimEvidenceCommand{
		MicroEventID: 7, DocumentVersionID: 11,
	})
	if err != nil {
		t.Fatalf("ScheduleAutomaticClaimEvidence(): %v", err)
	}
	if result.JobID != 101 || !result.Created || result.MicroEventID != 7 || result.DocumentVersionID != 11 {
		t.Fatalf("result = %#v", result)
	}
	if enqueuer.job.Kind != queue.KindExtractAutomaticClaimEvidence || enqueuer.job.UniqueKey != AutomaticClaimEvidenceUniqueKey(7, 11) ||
		!enqueuer.job.ScheduledAt.Equal(now) || enqueuer.job.MaxAttempts != 5 || enqueuer.job.Priority != 6 {
		t.Fatalf("job = %#v", enqueuer.job)
	}
	var args map[string]any
	if err := json.Unmarshal(enqueuer.job.DurableArgs, &args); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"micro_event_id", "document_version_id", "trace_id"}
	if reflect.TypeOf(automaticClaimEvidenceJobArgs{}).NumField() != len(wantKeys) || len(args) != len(wantKeys) {
		t.Fatalf("durable args = %#v", args)
	}
	for _, key := range wantKeys {
		if _, found := args[key]; !found {
			t.Fatalf("durable args missing %q: %#v", key, args)
		}
	}
	for _, forbidden := range []string{"event_version", "body", "plaintext", "object_key", "canonical_url", "token"} {
		if _, found := args[forbidden]; found {
			t.Fatalf("durable args leaked %q: %#v", forbidden, args)
		}
	}
}

func TestAutomaticClaimEvidenceHandlerResolvesCurrentEventVersionAndIsolatesDegradation(t *testing.T) {
	state := &eventapplication.EvidenceStateSnapshotDTO{ID: 17, EventVersion: 3}
	summary := &eventapplication.EvidenceSummaryDTO{ID: 19}
	extractor := &automaticClaimEvidenceExtractorFake{result: eventapplication.AutomaticClaimEvidenceResult{
		Status: "succeeded", ModelRunID: 23, EvidenceState: state, Summary: summary,
	}}
	refreshes := &automaticEvidenceRefreshSchedulerFake{}
	handler, err := newAutomaticClaimEvidenceHandler(extractor, refreshes)
	if err != nil {
		t.Fatal(err)
	}
	job := automaticClaimEvidenceTestJob([]byte(`{"micro_event_id":7,"document_version_id":11,"trace_id":""}`))
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle(): %v", err)
	}
	if extractor.command != (eventapplication.AutomaticClaimEvidenceCommand{MicroEventID: 7, DocumentVersionID: 11}) {
		t.Fatalf("extract command = %#v", extractor.command)
	}
	if refreshes.command != (eventapplication.ScheduleProductEventRefreshCommand{MicroEventID: 7, ExpectedEventVersion: 3}) {
		t.Fatalf("refresh command = %#v", refreshes.command)
	}

	extractor.result = eventapplication.AutomaticClaimEvidenceResult{Status: "degraded", ReasonCode: "external_model_not_authorized"}
	if err := handler.Handle(context.Background(), job); !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("degraded handler error = %v, want isolated permanent job failure", err)
	}
	extractor.result = eventapplication.AutomaticClaimEvidenceResult{Status: "degraded", ReasonCode: "ai_provider_timeout"}
	if err := handler.Handle(context.Background(), job); !errors.Is(err, queue.ErrRetryable) {
		t.Fatalf("pending-analysis handler error = %v, want retryable isolated failure", err)
	}

	unsafe := automaticClaimEvidenceTestJob([]byte(`{"micro_event_id":7,"document_version_id":11,"trace_id":"","body":"secret"}`))
	if err := handler.Handle(context.Background(), unsafe); !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("unsafe args error = %v, want permanent", err)
	}
}

type automaticEvidenceRefreshSchedulerFake struct {
	command eventapplication.ScheduleProductEventRefreshCommand
}

func (fake *automaticEvidenceRefreshSchedulerFake) ScheduleProductEventRefresh(_ context.Context,
	command eventapplication.ScheduleProductEventRefreshCommand) (eventapplication.ScheduleProductEventRefreshResult, error) {
	fake.command = command
	return eventapplication.ScheduleProductEventRefreshResult{MicroEventID: command.MicroEventID,
		MicroEventVersion: command.ExpectedEventVersion, JobID: 99, Created: true, Available: true}, nil
}

type automaticClaimEvidenceEnqueuerFake struct {
	job     queue.Job
	id      int64
	created bool
}

func (fake *automaticClaimEvidenceEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	fake.job = job
	return fake.id, fake.created, nil
}

type automaticClaimEvidenceExtractorFake struct {
	command eventapplication.AutomaticClaimEvidenceCommand
	result  eventapplication.AutomaticClaimEvidenceResult
	err     error
}

func (fake *automaticClaimEvidenceExtractorFake) Extract(_ context.Context,
	command eventapplication.AutomaticClaimEvidenceCommand) (eventapplication.AutomaticClaimEvidenceResult, error) {
	fake.command = command
	return fake.result, fake.err
}

func automaticClaimEvidenceTestJob(args []byte) queue.Job {
	return queue.Job{ID: 101, Kind: queue.KindExtractAutomaticClaimEvidence,
		UniqueKey: AutomaticClaimEvidenceUniqueKey(7, 11), DurableArgs: args,
		ScheduledAt: time.Now().UTC(), MaxAttempts: 5, Priority: 6}
}
