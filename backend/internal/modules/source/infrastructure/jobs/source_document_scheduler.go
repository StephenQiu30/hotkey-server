package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type sourceDocumentJobEnqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

// sourceDocumentGenerationJobArgs is the private queue wire record. Its two
// semantic identifiers are the complete producer contract; raw bytes, body
// text and object-store coordinates are intentionally unrepresentable.
type sourceDocumentGenerationJobArgs struct {
	EvidenceReferenceID int64  `json:"evidence_reference_id"`
	TraceID             string `json:"trace_id"`
}

type SourceDocumentGenerationScheduler struct {
	jobs sourceDocumentJobEnqueuer
}

var _ sourceapplication.SourceDocumentGenerationScheduler = (*SourceDocumentGenerationScheduler)(nil)

func NewSourceDocumentGenerationScheduler(jobs *queue.Store) (*SourceDocumentGenerationScheduler, error) {
	return newSourceDocumentGenerationScheduler(jobs)
}

func newSourceDocumentGenerationScheduler(jobs sourceDocumentJobEnqueuer) (*SourceDocumentGenerationScheduler, error) {
	if jobs == nil {
		return nil, fmt.Errorf("source document generation job enqueuer is required")
	}
	return &SourceDocumentGenerationScheduler{jobs: jobs}, nil
}

func (scheduler *SourceDocumentGenerationScheduler) Schedule(ctx context.Context, command sourceapplication.ScheduleSourceDocumentGenerationCommand) (sourceapplication.ScheduleSourceDocumentGenerationResult, error) {
	if scheduler == nil || scheduler.jobs == nil {
		return sourceapplication.ScheduleSourceDocumentGenerationResult{}, fmt.Errorf("source document generation scheduler is unavailable")
	}
	if err := command.Validate(); err != nil {
		return sourceapplication.ScheduleSourceDocumentGenerationResult{}, err
	}
	result := sourceapplication.ScheduleSourceDocumentGenerationResult{
		Receipts: make([]sourceapplication.SourceDocumentGenerationScheduleReceiptDTO, 0, len(command.EvidenceReferences)),
	}
	for _, reference := range command.EvidenceReferences {
		encoded, err := json.Marshal(sourceDocumentGenerationJobArgs{
			EvidenceReferenceID: reference.EvidenceReferenceID,
			TraceID:             command.TraceID,
		})
		if err != nil {
			return sourceapplication.ScheduleSourceDocumentGenerationResult{}, fmt.Errorf("encode source document generation job args")
		}
		jobID, created, err := scheduler.jobs.Enqueue(ctx, queue.Job{
			Kind:        queue.KindGenerateSourceDocument,
			UniqueKey:   SourceDocumentGenerationUniqueKey(reference.EvidenceReferenceID),
			DurableArgs: encoded,
			ScheduledAt: command.ScheduledAt.UTC(),
			MaxAttempts: 5,
			Priority:    3,
		})
		if err != nil {
			return sourceapplication.ScheduleSourceDocumentGenerationResult{}, fmt.Errorf("enqueue source document generation job: %w", err)
		}
		result.Receipts = append(result.Receipts, sourceapplication.SourceDocumentGenerationScheduleReceiptDTO{
			EvidenceReferenceID: reference.EvidenceReferenceID,
			JobID:               jobID,
			Created:             created,
		})
	}
	if err := sourceapplication.ValidateSourceDocumentGenerationScheduleResult(command, result); err != nil {
		return sourceapplication.ScheduleSourceDocumentGenerationResult{}, err
	}
	return result, nil
}

func SourceDocumentGenerationUniqueKey(evidenceReferenceID int64) string {
	if evidenceReferenceID <= 0 {
		return ""
	}
	return queue.StableJobHash(queue.KindGenerateSourceDocument, strconv.FormatInt(evidenceReferenceID, 10))
}
