package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

func TestPublishedDocumentMatchEvaluationSchedulerEnqueuesOnlyExactDocumentIdentity(t *testing.T) {
	t.Parallel()
	enqueuer := &publishedMatchJobEnqueuerFake{jobID: 91, created: true}
	scheduler, err := newPublishedDocumentMatchEvaluationScheduler(enqueuer, func() time.Time {
		return time.Date(2026, time.August, 10, 8, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := sharedrequestcontext.WithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	result, err := scheduler.SchedulePublishedDocumentMatchEvaluation(ctx, ingestionapplication.SchedulePublishedDocumentMatchEvaluationCommand{DocumentVersionID: 71})
	if err != nil {
		t.Fatalf("SchedulePublishedDocumentMatchEvaluation() error = %v", err)
	}
	if result.DocumentVersionID != 71 || result.JobID != 91 || !result.Created || enqueuer.calls != 1 {
		t.Fatalf("schedule result/calls = %#v/%d", result, enqueuer.calls)
	}
	job := enqueuer.job
	if job.Kind != queue.KindEvaluatePublishedDocumentMatches || job.UniqueKey != PublishedDocumentMatchEvaluationUniqueKey(71) ||
		job.Payload != (queue.Payload{}) || job.MaxAttempts != 5 || job.Priority != 4 {
		t.Fatalf("scheduled job = %#v", job)
	}
	var args map[string]any
	if err := json.Unmarshal(job.DurableArgs, &args); err != nil {
		t.Fatal(err)
	}
	wantKeys := map[string]struct{}{"document_version_id": {}, "trace_id": {}}
	if len(args) != len(wantKeys) {
		t.Fatalf("durable args = %s", job.DurableArgs)
	}
	for key := range args {
		if _, allowed := wantKeys[key]; !allowed {
			t.Fatalf("durable args expose %q: %s", key, job.DurableArgs)
		}
	}
	encoded := string(job.DurableArgs)
	for _, forbidden := range []string{"body", "plaintext", "markdown", "object_key", "monitor_id", "profile_id"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("durable args leak %q: %s", forbidden, encoded)
		}
	}
}

func TestPublishedDocumentMatchEvaluationHandlerDecodesStrictArgsAndPropagatesTrace(t *testing.T) {
	t.Parallel()
	evaluator := &publishedMatchEvaluatorFake{}
	handler, err := newPublishedDocumentMatchEvaluationHandler(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	job := publishedMatchEvaluationJob([]byte(`{"document_version_id":71,"trace_id":"0123456789abcdef0123456789abcdef"}`))
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if evaluator.calls != 1 || evaluator.command.DocumentVersionID != 71 || evaluator.traceID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("evaluation call = %d %#v trace=%q", evaluator.calls, evaluator.command, evaluator.traceID)
	}
	if reflect.TypeOf(publishedDocumentMatchEvaluationJobArgs{}).NumField() != 2 {
		t.Fatal("published match job args grew beyond exact document identity and trace")
	}
}

func TestPublishedDocumentMatchEvaluationHandlerRejectsUnsafeArgsAndClassifiesErrors(t *testing.T) {
	t.Parallel()
	unsafe := []string{
		`{"document_version_id":0,"trace_id":""}`,
		`{"document_version_id":71,"trace_id":"abc"}`,
		`{"document_version_id":71,"trace_id":"","body":"secret"}`,
		`{"document_version_id":71,"trace_id":"","monitor_id":3}`,
		`{"document_version_id":71,"trace_id":""}{}`,
	}
	for _, encoded := range unsafe {
		evaluator := &publishedMatchEvaluatorFake{}
		handler, _ := newPublishedDocumentMatchEvaluationHandler(evaluator)
		if err := handler.Handle(context.Background(), publishedMatchEvaluationJob([]byte(encoded))); !queue.IsPermanent(err) || evaluator.calls != 0 {
			t.Fatalf("unsafe args %q error/calls = %v/%d", encoded, err, evaluator.calls)
		}
	}

	for _, test := range []struct {
		cause error
		check func(error) bool
	}{{sharedrepository.ErrUnavailable, queue.IsRetryable}, {sharedrepository.ErrConflict, queue.IsPermanent}, {context.Canceled, queue.IsCancelled}} {
		evaluator := &publishedMatchEvaluatorFake{err: test.cause}
		handler, _ := newPublishedDocumentMatchEvaluationHandler(evaluator)
		err := handler.Handle(context.Background(), publishedMatchEvaluationJob([]byte(`{"document_version_id":71,"trace_id":""}`)))
		if !test.check(err) || !errors.Is(err, test.cause) {
			t.Fatalf("classified error = %v for %v", err, test.cause)
		}
	}
}

func publishedMatchEvaluationJob(args []byte) queue.Job {
	return queue.Job{
		Kind: queue.KindEvaluatePublishedDocumentMatches, UniqueKey: "published-document-match-71", DurableArgs: args,
		ScheduledAt: time.Date(2026, time.August, 10, 8, 0, 0, 0, time.UTC), MaxAttempts: 5, Priority: 4,
	}
}

type publishedMatchJobEnqueuerFake struct {
	job     queue.Job
	jobID   int64
	created bool
	err     error
	calls   int
}

func (enqueuer *publishedMatchJobEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	enqueuer.calls++
	enqueuer.job = job
	return enqueuer.jobID, enqueuer.created, enqueuer.err
}

type publishedMatchEvaluatorFake struct {
	command ingestionapplication.EvaluatePublishedMatchesForDocumentCommand
	traceID string
	err     error
	calls   int
}

func (evaluator *publishedMatchEvaluatorFake) EvaluateForDocument(ctx context.Context, command ingestionapplication.EvaluatePublishedMatchesForDocumentCommand) (ingestionapplication.EvaluatePublishedMatchesForDocumentResult, error) {
	evaluator.calls++
	evaluator.command = command
	evaluator.traceID = sharedrequestcontext.TraceID(ctx)
	return ingestionapplication.EvaluatePublishedMatchesForDocumentResult{DocumentVersionID: command.DocumentVersionID}, evaluator.err
}
