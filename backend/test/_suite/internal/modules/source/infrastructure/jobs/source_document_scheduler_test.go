package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type sourceDocumentJobEnqueuerFake struct {
	jobs       []queue.Job
	created    []bool
	failAtCall int
}

func (fake *sourceDocumentJobEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	fake.jobs = append(fake.jobs, job)
	if fake.failAtCall > 0 && len(fake.jobs) == fake.failAtCall {
		return 0, false, errors.New("queue unavailable")
	}
	created := true
	if len(fake.created) >= len(fake.jobs) {
		created = fake.created[len(fake.jobs)-1]
	}
	return int64(700 + len(fake.jobs)), created, nil
}

func TestSourceDocumentGenerationSchedulerMapsExactSemanticJobs(t *testing.T) {
	t.Parallel()

	enqueuer := &sourceDocumentJobEnqueuerFake{created: []bool{true, false}}
	scheduler, err := newSourceDocumentGenerationScheduler(enqueuer)
	if err != nil {
		t.Fatal(err)
	}
	scheduledAt := time.Date(2026, time.August, 9, 12, 30, 0, 0, time.UTC)
	traceID := "0123456789abcdef0123456789abcdef"
	command := sourceapplication.ScheduleSourceDocumentGenerationCommand{
		EvidenceReferences: []sourceapplication.CommittedEvidenceReferenceDTO{
			{EvidenceReferenceID: 71, SourceObservationID: 31, EvidenceSnapshotID: 11, Usage: "document_source"},
			{EvidenceReferenceID: 72, SourceObservationID: 32, EvidenceSnapshotID: 11, Usage: "document_source"},
		},
		TraceID: traceID, ScheduledAt: scheduledAt,
	}

	result, err := scheduler.Schedule(context.Background(), command)
	if err != nil {
		t.Fatalf("Schedule(): %v", err)
	}
	wantReceipts := []sourceapplication.SourceDocumentGenerationScheduleReceiptDTO{
		{EvidenceReferenceID: 71, JobID: 701, Created: true},
		{EvidenceReferenceID: 72, JobID: 702, Created: false},
	}
	if !reflect.DeepEqual(result.Receipts, wantReceipts) {
		t.Fatalf("Schedule() receipts = %#v, want %#v", result.Receipts, wantReceipts)
	}
	if len(enqueuer.jobs) != 2 {
		t.Fatalf("enqueued jobs = %d, want 2", len(enqueuer.jobs))
	}
	for index, job := range enqueuer.jobs {
		referenceID := command.EvidenceReferences[index].EvidenceReferenceID
		if job.Kind != queue.KindGenerateSourceDocument || job.Payload != (queue.Payload{}) || job.ScheduledAt != scheduledAt ||
			job.MaxAttempts != 5 || job.Priority != 3 || job.UniqueKey != SourceDocumentGenerationUniqueKey(referenceID) {
			t.Fatalf("job %d = %#v", index, job)
		}
		var args map[string]any
		if err := json.Unmarshal(job.DurableArgs, &args); err != nil {
			t.Fatalf("decode job %d args: %v", index, err)
		}
		if len(args) != 2 || args["evidence_reference_id"] != float64(referenceID) || args["trace_id"] != traceID {
			t.Fatalf("job %d args = %#v", index, args)
		}
		encoded := string(job.DurableArgs)
		for _, forbidden := range []string{"body", "raw", "payload", "object_key", "snapshot_id", "observation_id"} {
			if strings.Contains(encoded, forbidden) {
				t.Fatalf("job %d args leaked %q: %s", index, forbidden, encoded)
			}
		}
	}
	if enqueuer.jobs[0].UniqueKey == enqueuer.jobs[1].UniqueKey {
		t.Fatal("different evidence references shared a unique key")
	}
	withoutTrace := command
	withoutTrace.TraceID = ""
	withoutTrace.EvidenceReferences = withoutTrace.EvidenceReferences[:1]
	result, err = scheduler.Schedule(context.Background(), withoutTrace)
	if err != nil || len(result.Receipts) != 1 {
		t.Fatalf("Schedule() without trace = %#v/%v", result, err)
	}
	var args map[string]any
	if err := json.Unmarshal(enqueuer.jobs[2].DurableArgs, &args); err != nil {
		t.Fatal(err)
	}
	if value, found := args["trace_id"]; !found || value != "" {
		t.Fatalf("empty trace contract = %#v", args)
	}
}

func TestSourceDocumentGenerationSchedulerEmptyAndFailureContracts(t *testing.T) {
	t.Parallel()

	emptyEnqueuer := &sourceDocumentJobEnqueuerFake{}
	scheduler, err := newSourceDocumentGenerationScheduler(emptyEnqueuer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := scheduler.Schedule(context.Background(), sourceapplication.ScheduleSourceDocumentGenerationCommand{
		EvidenceReferences: []sourceapplication.CommittedEvidenceReferenceDTO{},
		ScheduledAt:        time.Now().UTC(),
	})
	if err != nil || result.Receipts == nil || len(result.Receipts) != 0 || len(emptyEnqueuer.jobs) != 0 {
		t.Fatalf("empty Schedule() = %#v/%v, jobs=%d", result, err, len(emptyEnqueuer.jobs))
	}

	failingEnqueuer := &sourceDocumentJobEnqueuerFake{failAtCall: 2}
	failingScheduler, err := newSourceDocumentGenerationScheduler(failingEnqueuer)
	if err != nil {
		t.Fatal(err)
	}
	_, err = failingScheduler.Schedule(context.Background(), sourceapplication.ScheduleSourceDocumentGenerationCommand{
		EvidenceReferences: []sourceapplication.CommittedEvidenceReferenceDTO{
			{EvidenceReferenceID: 81, SourceObservationID: 41, EvidenceSnapshotID: 21, Usage: "document_source"},
			{EvidenceReferenceID: 82, SourceObservationID: 42, EvidenceSnapshotID: 21, Usage: "document_source"},
		},
		ScheduledAt: time.Now().UTC(),
	})
	if err == nil || len(failingEnqueuer.jobs) != 2 {
		t.Fatalf("failing Schedule() = %v, calls=%d", err, len(failingEnqueuer.jobs))
	}
}

func TestSourceDocumentGenerationSchedulerRejectsUnsafeApplicationCommands(t *testing.T) {
	t.Parallel()

	scheduler, err := newSourceDocumentGenerationScheduler(&sourceDocumentJobEnqueuerFake{})
	if err != nil {
		t.Fatal(err)
	}
	valid := sourceapplication.ScheduleSourceDocumentGenerationCommand{
		EvidenceReferences: []sourceapplication.CommittedEvidenceReferenceDTO{{EvidenceReferenceID: 91, SourceObservationID: 51, EvidenceSnapshotID: 31, Usage: "document_source"}},
		ScheduledAt:        time.Now().UTC(),
	}
	tests := []struct {
		name   string
		mutate func(*sourceapplication.ScheduleSourceDocumentGenerationCommand)
	}{
		{name: "zero schedule time", mutate: func(command *sourceapplication.ScheduleSourceDocumentGenerationCommand) {
			command.ScheduledAt = time.Time{}
		}},
		{name: "invalid trace", mutate: func(command *sourceapplication.ScheduleSourceDocumentGenerationCommand) {
			command.TraceID = "not-a-trace"
		}},
		{name: "zero reference", mutate: func(command *sourceapplication.ScheduleSourceDocumentGenerationCommand) {
			command.EvidenceReferences[0].EvidenceReferenceID = 0
		}},
		{name: "zero observation", mutate: func(command *sourceapplication.ScheduleSourceDocumentGenerationCommand) {
			command.EvidenceReferences[0].SourceObservationID = 0
		}},
		{name: "zero snapshot", mutate: func(command *sourceapplication.ScheduleSourceDocumentGenerationCommand) {
			command.EvidenceReferences[0].EvidenceSnapshotID = 0
		}},
		{name: "duplicate reference", mutate: func(command *sourceapplication.ScheduleSourceDocumentGenerationCommand) {
			command.EvidenceReferences = append(command.EvidenceReferences, command.EvidenceReferences[0])
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := valid
			command.EvidenceReferences = append([]sourceapplication.CommittedEvidenceReferenceDTO(nil), valid.EvidenceReferences...)
			test.mutate(&command)
			if _, err := scheduler.Schedule(context.Background(), command); err == nil {
				t.Fatal("Schedule() accepted unsafe command")
			}
		})
	}
}
