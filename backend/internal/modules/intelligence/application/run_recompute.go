package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// AIRunReader exposes only the execution fact required to authorize a
// recompute. Provider inputs and credentials stay behind their owning module.
type AIRunReader interface {
	FindRun(context.Context, int64) (domain.Run, error)
}

// AIRunRecomputeScheduler is the only application-to-queue boundary. The
// durable command contains the immutable run identity and nothing else.
type AIRunRecomputeScheduler interface {
	ScheduleAIRunRecompute(context.Context, int64) (int64, bool, error)
}

type AIRunRecomputeResult struct {
	RunID, JobID int64
	Created      bool
}

type AIRunRecomputeService struct {
	runs      AIRunReader
	scheduler AIRunRecomputeScheduler
}

func NewAIRunRecomputeService(runs AIRunReader, scheduler AIRunRecomputeScheduler) (*AIRunRecomputeService, error) {
	if runs == nil || scheduler == nil {
		return nil, fmt.Errorf("AI run recompute dependencies are required")
	}
	return &AIRunRecomputeService{runs: runs, scheduler: scheduler}, nil
}

// Schedule accepts only terminal unsuccessful runs produced by a durable
// owning job. The recovery worker will reread this fact before reactivation.
func (service *AIRunRecomputeService) Schedule(ctx context.Context, runID int64) (AIRunRecomputeResult, error) {
	if service == nil || service.runs == nil || service.scheduler == nil {
		return AIRunRecomputeResult{}, sharedrepository.ErrUnavailable
	}
	if runID <= 0 {
		return AIRunRecomputeResult{}, fmt.Errorf("%w: invalid AI run id", sharedrepository.ErrInvalidInput)
	}
	run, err := service.runs.FindRun(ctx, runID)
	if err != nil {
		return AIRunRecomputeResult{}, err
	}
	if (run.Status != domain.RunStatusFailed && run.Status != domain.RunStatusCancelled) || run.OwningJobID == nil || *run.OwningJobID <= 0 {
		return AIRunRecomputeResult{}, fmt.Errorf("%w: AI run cannot be recomputed", sharedrepository.ErrConflict)
	}
	jobID, created, err := service.scheduler.ScheduleAIRunRecompute(ctx, run.ID)
	if err != nil {
		return AIRunRecomputeResult{}, err
	}
	return AIRunRecomputeResult{RunID: run.ID, JobID: jobID, Created: created}, nil
}
