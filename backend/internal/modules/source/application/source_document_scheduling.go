package application

import (
	"context"
	"fmt"
	"time"
)

// CommittedEvidenceReferenceDTO is the exact immutable locator receipt
// produced by committing an observation-to-snapshot relationship. It contains
// identifiers only; selected bytes and object-store coordinates remain behind
// Source-owned read ports.
type CommittedEvidenceReferenceDTO struct {
	EvidenceReferenceID int64
	SourceObservationID int64
	EvidenceSnapshotID  int64
}

func (reference CommittedEvidenceReferenceDTO) Validate() error {
	if reference.EvidenceReferenceID <= 0 || reference.SourceObservationID <= 0 || reference.EvidenceSnapshotID <= 0 {
		return fmt.Errorf("committed evidence reference identity is invalid")
	}
	return nil
}

// CommitEvidenceSnapshotResult exposes the available snapshot together with
// every exact locator committed in the same PostgreSQL transaction.
type CommitEvidenceSnapshotResult struct {
	Snapshot           PersistedEvidenceSnapshotDTO
	EvidenceReferences []CommittedEvidenceReferenceDTO
}

type ScheduleSourceDocumentGenerationCommand struct {
	EvidenceReferences []CommittedEvidenceReferenceDTO
	TraceID            string
	ScheduledAt        time.Time
}

func (command ScheduleSourceDocumentGenerationCommand) Validate() error {
	if command.EvidenceReferences == nil || command.ScheduledAt.IsZero() || !validSourceDocumentTraceID(command.TraceID) {
		return fmt.Errorf("source document generation schedule command is invalid")
	}
	seen := make(map[int64]struct{}, len(command.EvidenceReferences))
	for _, reference := range command.EvidenceReferences {
		if err := reference.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[reference.EvidenceReferenceID]; duplicate {
			return fmt.Errorf("source document generation schedule contains a duplicate evidence reference")
		}
		seen[reference.EvidenceReferenceID] = struct{}{}
	}
	return nil
}

type SourceDocumentGenerationScheduleReceiptDTO struct {
	EvidenceReferenceID int64
	JobID               int64
	Created             bool
}

type ScheduleSourceDocumentGenerationResult struct {
	Receipts []SourceDocumentGenerationScheduleReceiptDTO
}

// SourceDocumentGenerationScheduler is the Source Application boundary for
// creating durable work. Implementations must participate in a caller-owned
// transaction; Application code is deliberately unaware of queue envelopes.
type SourceDocumentGenerationScheduler interface {
	Schedule(context.Context, ScheduleSourceDocumentGenerationCommand) (ScheduleSourceDocumentGenerationResult, error)
}

func ValidateSourceDocumentGenerationScheduleResult(command ScheduleSourceDocumentGenerationCommand, result ScheduleSourceDocumentGenerationResult) error {
	if err := command.Validate(); err != nil {
		return err
	}
	if result.Receipts == nil || len(result.Receipts) != len(command.EvidenceReferences) {
		return fmt.Errorf("source document generation schedule receipt set is incomplete")
	}
	expected := make(map[int64]struct{}, len(command.EvidenceReferences))
	for _, reference := range command.EvidenceReferences {
		expected[reference.EvidenceReferenceID] = struct{}{}
	}
	seen := make(map[int64]struct{}, len(result.Receipts))
	for _, receipt := range result.Receipts {
		if receipt.EvidenceReferenceID <= 0 || receipt.JobID <= 0 {
			return fmt.Errorf("source document generation schedule receipt is invalid")
		}
		if _, found := expected[receipt.EvidenceReferenceID]; !found {
			return fmt.Errorf("source document generation schedule receipt contains an unexpected reference")
		}
		if _, duplicate := seen[receipt.EvidenceReferenceID]; duplicate {
			return fmt.Errorf("source document generation schedule receipt contains a duplicate reference")
		}
		seen[receipt.EvidenceReferenceID] = struct{}{}
	}
	return nil
}

func validSourceDocumentTraceID(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 32 {
		return false
	}
	for index := range value {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return false
		}
	}
	return true
}
