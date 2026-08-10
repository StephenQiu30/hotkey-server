package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestAcceptedDocumentMatchProjectionSchedulerPersistsOnlyExactMatchIdentity(t *testing.T) {
	now := time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC)
	enqueuer := &acceptedMatchProjectionEnqueuerFake{id: 91, created: true}
	scheduler, err := newAcceptedDocumentMatchProjectionScheduler(enqueuer, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := scheduler.ScheduleAcceptedDocumentMatchProjection(context.Background(),
		ingestionapplication.ScheduleAcceptedDocumentMatchProjectionCommand{
			DocumentMatchDecisionID: 23, DocumentVersionID: 17, EffectiveSequence: 2,
		})
	if err != nil {
		t.Fatalf("ScheduleAcceptedDocumentMatchProjection(): %v", err)
	}
	if result.JobID != 91 || !result.Created || result.DocumentMatchDecisionID != 23 || result.DocumentVersionID != 17 || result.EffectiveSequence != 2 {
		t.Fatalf("result = %#v", result)
	}
	if enqueuer.job.Kind != queue.KindProjectAcceptedDocumentMatch ||
		enqueuer.job.UniqueKey != AcceptedDocumentMatchProjectionUniqueKey(23, 2) || !enqueuer.job.ScheduledAt.Equal(now) {
		t.Fatalf("job = %#v", enqueuer.job)
	}
	var args map[string]any
	if err := json.Unmarshal(enqueuer.job.DurableArgs, &args); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"document_match_decision_id", "document_version_id", "effective_sequence", "trace_id"}
	if reflect.TypeOf(acceptedDocumentMatchProjectionJobArgs{}).NumField() != len(wantKeys) || len(args) != len(wantKeys) {
		t.Fatalf("durable args = %#v", args)
	}
	for _, key := range wantKeys {
		if _, found := args[key]; !found {
			t.Fatalf("durable args missing %q: %#v", key, args)
		}
	}
	for _, forbidden := range []string{"body", "plaintext", "object_key", "canonical_url", "note"} {
		if _, found := args[forbidden]; found {
			t.Fatalf("durable args leaked %q: %#v", forbidden, args)
		}
	}
}

func TestAcceptedDocumentMatchProjectionHandlerConsumesExactIdentityAndRejectsUnsafeArgs(t *testing.T) {
	consumer := &acceptedMatchProjectionConsumerFake{}
	handler, err := NewAcceptedDocumentMatchProjectionHandler(consumer)
	if err != nil {
		t.Fatal(err)
	}
	args := []byte(`{"document_match_decision_id":23,"document_version_id":17,"effective_sequence":2,"trace_id":""}`)
	job := queue.Job{ID: 91, Kind: queue.KindProjectAcceptedDocumentMatch,
		UniqueKey: AcceptedDocumentMatchProjectionUniqueKey(23, 2), DurableArgs: args,
		ScheduledAt: time.Now().UTC(), MaxAttempts: 5, Priority: 4}
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle(): %v", err)
	}
	if consumer.command != (ingestionapplication.ConsumeAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: 23, DocumentVersionID: 17,
	}) {
		t.Fatalf("consumer command = %#v", consumer.command)
	}
	unsafe := job
	unsafe.DurableArgs = []byte(`{"document_match_decision_id":23,"document_version_id":17,"effective_sequence":2,"trace_id":"","body":"secret"}`)
	if err := handler.Handle(context.Background(), unsafe); !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("unsafe args error = %v, want permanent", err)
	}
}

type acceptedMatchProjectionEnqueuerFake struct {
	job     queue.Job
	id      int64
	created bool
	err     error
}

func (fake *acceptedMatchProjectionEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	fake.job = job
	return fake.id, fake.created, fake.err
}

type acceptedMatchProjectionConsumerFake struct {
	command ingestionapplication.ConsumeAcceptedDocumentMatchCommand
	err     error
}

func (fake *acceptedMatchProjectionConsumerFake) ConsumeAcceptedDocumentMatch(_ context.Context,
	command ingestionapplication.ConsumeAcceptedDocumentMatchCommand) (ingestionapplication.ConsumeAcceptedDocumentMatchResult, error) {
	fake.command = command
	return ingestionapplication.ConsumeAcceptedDocumentMatchResult{
		DocumentMatchDecisionID: command.DocumentMatchDecisionID,
		DocumentVersionID:       command.DocumentVersionID,
	}, fake.err
}
