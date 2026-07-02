package jobs

import (
	"context"
	"log"

	"github.com/StephenQiu30/hotkey-server/internal/knowledge"
)

// ChangeScanner provides writeback changes to be applied.
type ChangeScanner interface {
	Scan(ctx context.Context) ([]knowledge.WritebackChange, error)
}

// KnowledgeService applies a validated writeback change.
type KnowledgeService interface {
	ApplyChange(ctx context.Context, change knowledge.WritebackChange, conflict knowledge.ConflictInput) error
}

// WritebackResult reports the outcome of a writeback job run.
type WritebackResult struct {
	ScannedCount  int
	AppliedCount  int
	FailedCount   int
	SkippedCount  int
}

// ApplyKnowledgeWritebackJob scans for pending writeback changes and applies them.
type ApplyKnowledgeWritebackJob struct {
	scanner ChangeScanner
	svc     KnowledgeService
}

// NewApplyKnowledgeWritebackJob creates a new ApplyKnowledgeWritebackJob.
func NewApplyKnowledgeWritebackJob(scanner ChangeScanner, svc KnowledgeService) *ApplyKnowledgeWritebackJob {
	return &ApplyKnowledgeWritebackJob{
		scanner: scanner,
		svc:     svc,
	}
}

// Run executes the writeback job, scanning for changes and applying them.
func (j *ApplyKnowledgeWritebackJob) Run(ctx context.Context) (*WritebackResult, error) {
	changes, err := j.scanner.Scan(ctx)
	if err != nil {
		return nil, err
	}

	result := &WritebackResult{
		ScannedCount: len(changes),
	}

	for _, change := range changes {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		err := j.svc.ApplyChange(ctx, change, knowledge.ConflictInput{})
		if err != nil {
			log.Printf("writeback: apply change for %s/%s on %q: %v",
				change.ObjectType, change.SourcePath, change.FieldName, err)
			result.FailedCount++
		} else {
			result.AppliedCount++
		}
	}

	return result, nil
}
