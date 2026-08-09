package application

import (
	"testing"
	"time"
)

func TestSourceDocumentGenerationScheduleResultRequiresExactReceiptSet(t *testing.T) {
	t.Parallel()

	command := ScheduleSourceDocumentGenerationCommand{
		EvidenceReferences: []CommittedEvidenceReferenceDTO{
			{EvidenceReferenceID: 11, SourceObservationID: 21, EvidenceSnapshotID: 31},
			{EvidenceReferenceID: 12, SourceObservationID: 22, EvidenceSnapshotID: 31},
		},
		TraceID: "0123456789abcdef0123456789abcdef", ScheduledAt: time.Now().UTC(),
	}
	valid := ScheduleSourceDocumentGenerationResult{Receipts: []SourceDocumentGenerationScheduleReceiptDTO{
		{EvidenceReferenceID: 11, JobID: 41, Created: true},
		{EvidenceReferenceID: 12, JobID: 42, Created: false},
	}}
	if err := ValidateSourceDocumentGenerationScheduleResult(command, valid); err != nil {
		t.Fatalf("valid result: %v", err)
	}
	tests := []struct {
		name   string
		result ScheduleSourceDocumentGenerationResult
	}{
		{name: "nil receipts", result: ScheduleSourceDocumentGenerationResult{}},
		{name: "missing receipt", result: ScheduleSourceDocumentGenerationResult{Receipts: valid.Receipts[:1]}},
		{name: "duplicate receipt", result: ScheduleSourceDocumentGenerationResult{Receipts: []SourceDocumentGenerationScheduleReceiptDTO{valid.Receipts[0], valid.Receipts[0]}}},
		{name: "unexpected receipt", result: ScheduleSourceDocumentGenerationResult{Receipts: []SourceDocumentGenerationScheduleReceiptDTO{valid.Receipts[0], {EvidenceReferenceID: 99, JobID: 43}}}},
		{name: "missing job identity", result: ScheduleSourceDocumentGenerationResult{Receipts: []SourceDocumentGenerationScheduleReceiptDTO{valid.Receipts[0], {EvidenceReferenceID: 12}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateSourceDocumentGenerationScheduleResult(command, test.result); err == nil {
				t.Fatal("result validation accepted a non-exact receipt set")
			}
		})
	}
}

func TestSourceDocumentGenerationScheduleAcceptsCanonicalEmptySet(t *testing.T) {
	t.Parallel()

	command := ScheduleSourceDocumentGenerationCommand{
		EvidenceReferences: []CommittedEvidenceReferenceDTO{}, ScheduledAt: time.Now().UTC(),
	}
	result := ScheduleSourceDocumentGenerationResult{Receipts: []SourceDocumentGenerationScheduleReceiptDTO{}}
	if err := ValidateSourceDocumentGenerationScheduleResult(command, result); err != nil {
		t.Fatalf("canonical empty schedule: %v", err)
	}
}
