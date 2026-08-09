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

func TestSourceDocumentGenerationHandlerUsesOnlyEvidenceReferenceAndTrace(t *testing.T) {
	t.Parallel()

	generator := &sourceDocumentGeneratorFake{}
	handler, err := newSourceDocumentGenerationHandler(generator)
	if err != nil {
		t.Fatalf("newSourceDocumentGenerationHandler() error = %v", err)
	}
	traceID := "0123456789abcdef0123456789abcdef"
	args, err := json.Marshal(GenerateSourceDocumentJobArgs{EvidenceReferenceID: 71, TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	job := sourceDocumentGenerationJob(args)
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if generator.calls != 1 || generator.command.EvidenceReferenceID != 71 || generator.traceID != traceID {
		t.Fatalf("generator call = %d %#v trace=%q", generator.calls, generator.command, generator.traceID)
	}
	if got := string(args); strings.Contains(got, "body") || strings.Contains(got, "raw") || strings.Contains(got, "object_key") || strings.Contains(got, "selected") {
		t.Fatalf("durable args leaked evidence bytes or object identity: %s", got)
	}
	argsType := reflect.TypeOf(GenerateSourceDocumentJobArgs{})
	if argsType.NumField() != 2 || argsType.Field(0).Name != "EvidenceReferenceID" || argsType.Field(1).Name != "TraceID" {
		t.Fatalf("GenerateSourceDocumentJobArgs shape = %#v", argsType)
	}
}

func TestSourceDocumentGenerationHandlerRejectsUnknownOrUnsafeArgsPermanently(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args string
	}{
		{name: "missing evidence reference", args: `{"trace_id":""}`},
		{name: "zero evidence reference", args: `{"evidence_reference_id":0,"trace_id":""}`},
		{name: "uppercase trace", args: `{"evidence_reference_id":71,"trace_id":"0123456789ABCDEF0123456789ABCDEF"}`},
		{name: "short trace", args: `{"evidence_reference_id":71,"trace_id":"abc"}`},
		{name: "body", args: `{"evidence_reference_id":71,"trace_id":"","body":"secret"}`},
		{name: "raw bytes", args: `{"evidence_reference_id":71,"trace_id":"","raw":"secret"}`},
		{name: "object key", args: `{"evidence_reference_id":71,"trace_id":"","object_key":"raw/source/secret"}`},
		{name: "trailing value", args: `{"evidence_reference_id":71,"trace_id":""}{}`},
		{name: "array", args: `[{"evidence_reference_id":71,"trace_id":""}]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generator := &sourceDocumentGeneratorFake{}
			handler, err := newSourceDocumentGenerationHandler(generator)
			if err != nil {
				t.Fatal(err)
			}
			err = handler.Handle(context.Background(), sourceDocumentGenerationJob([]byte(test.args)))
			if !queue.IsPermanent(err) || generator.calls != 0 {
				t.Fatalf("Handle() error/calls = %v/%d, want permanent/0", err, generator.calls)
			}
		})
	}
}

func TestSourceDocumentGenerationHandlerClassifiesUseCaseErrors(t *testing.T) {
	t.Parallel()

	args := []byte(`{"evidence_reference_id":71,"trace_id":""}`)
	tests := []struct {
		name       string
		cause      error
		classified func(error) bool
	}{
		{name: "invalid immutable fact", cause: sharedrepository.ErrInvalidInput, classified: queue.IsPermanent},
		{name: "rights conflict", cause: sharedrepository.ErrConflict, classified: queue.IsPermanent},
		{name: "dependency unavailable", cause: sharedrepository.ErrUnavailable, classified: queue.IsRetryable},
		{name: "cancelled attempt", cause: context.Canceled, classified: queue.IsCancelled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generator := &sourceDocumentGeneratorFake{err: test.cause}
			handler, err := newSourceDocumentGenerationHandler(generator)
			if err != nil {
				t.Fatal(err)
			}
			err = handler.Handle(context.Background(), sourceDocumentGenerationJob(args))
			if !test.classified(err) || !errors.Is(err, test.cause) || generator.calls != 1 {
				t.Fatalf("Handle() error/calls = %v/%d", err, generator.calls)
			}
		})
	}
}

func TestSourceDocumentGenerationHandlerRejectsWrongEnvelopeBeforeUseCase(t *testing.T) {
	t.Parallel()

	generator := &sourceDocumentGeneratorFake{}
	handler, err := newSourceDocumentGenerationHandler(generator)
	if err != nil {
		t.Fatal(err)
	}
	job := sourceDocumentGenerationJob([]byte(`{"evidence_reference_id":71,"trace_id":""}`))
	job.Kind = queue.KindNormalizeContent
	if err := handler.Handle(context.Background(), job); !queue.IsPermanent(err) || generator.calls != 0 {
		t.Fatalf("wrong-kind Handle() = %v calls=%d", err, generator.calls)
	}
}

func sourceDocumentGenerationJob(args []byte) queue.Job {
	return queue.Job{
		Kind: queue.KindGenerateSourceDocument, UniqueKey: "source-document-71", DurableArgs: append([]byte(nil), args...),
		ScheduledAt: time.Date(2026, time.August, 9, 13, 0, 0, 0, time.UTC), MaxAttempts: 3, Priority: 3,
	}
}

type sourceDocumentGeneratorFake struct {
	command ingestionapplication.GenerateSourceDocumentCommand
	traceID string
	err     error
	calls   int
}

func (generator *sourceDocumentGeneratorFake) Generate(ctx context.Context, command ingestionapplication.GenerateSourceDocumentCommand) (ingestionapplication.GenerateSourceDocumentResult, error) {
	generator.calls++
	generator.command = command
	generator.traceID = sharedrequestcontext.TraceID(ctx)
	return ingestionapplication.GenerateSourceDocumentResult{}, generator.err
}
