package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type AIRunReader interface {
	FindRun(context.Context, int64) (intelligencedomain.Run, error)
}

// AIRunRecomputeJobArgs is intentionally a single-field durable command.
type AIRunRecomputeJobArgs struct {
	RunID int64 `json:"run_id"`
}

func EncodeAIRunRecomputeJobArgs(runID int64) (json.RawMessage, error) {
	if runID <= 0 {
		return nil, fmt.Errorf("%w: invalid AI run id", sharedrepository.ErrInvalidInput)
	}
	encoded, err := json.Marshal(AIRunRecomputeJobArgs{RunID: runID})
	if err != nil {
		return nil, fmt.Errorf("encode AI run recompute args: %w", err)
	}
	return encoded, nil
}

func DecodeAIRunRecomputeJobArgs(raw json.RawMessage) (AIRunRecomputeJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var args AIRunRecomputeJobArgs
	if err := decoder.Decode(&args); err != nil {
		return AIRunRecomputeJobArgs{}, fmt.Errorf("decode AI run recompute args: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return AIRunRecomputeJobArgs{}, fmt.Errorf("decode AI run recompute args: trailing JSON")
	}
	if args.RunID <= 0 {
		return AIRunRecomputeJobArgs{}, fmt.Errorf("decode AI run recompute args: invalid run id")
	}
	return args, nil
}

type AIRunRecomputeScheduler struct {
	jobs *queue.Store
	now  func() time.Time
}

func NewAIRunRecomputeScheduler(jobs *queue.Store) *AIRunRecomputeScheduler {
	return &AIRunRecomputeScheduler{jobs: jobs, now: func() time.Time { return time.Now().UTC() }}
}

func (scheduler *AIRunRecomputeScheduler) ScheduleAIRunRecompute(ctx context.Context, runID int64) (int64, bool, error) {
	if scheduler == nil || scheduler.jobs == nil {
		return 0, false, sharedrepository.ErrUnavailable
	}
	args, err := EncodeAIRunRecomputeJobArgs(runID)
	if err != nil {
		return 0, false, err
	}
	jobID, created, err := scheduler.jobs.Enqueue(ctx, queue.Job{
		Kind: queue.KindRecomputeAIRun, UniqueKey: queue.StableJobHash(queue.KindRecomputeAIRun, fmt.Sprint(runID)),
		DurableArgs: args, ScheduledAt: scheduler.now(), MaxAttempts: 5, Priority: 4,
	})
	if err != nil || created {
		return jobID, created, err
	}
	activation, err := scheduler.jobs.ReactivateByID(ctx, jobID)
	if err != nil {
		return 0, false, err
	}
	return activation.ID, false, nil
}

type AIRunRecomputeHandler struct {
	runs AIRunReader
	jobs *queue.Store
}

func NewAIRunRecomputeHandler(runs AIRunReader, jobs *queue.Store) *AIRunRecomputeHandler {
	return &AIRunRecomputeHandler{runs: runs, jobs: jobs}
}

func (handler *AIRunRecomputeHandler) Handle(ctx context.Context, job queue.Job) error {
	if handler == nil || handler.runs == nil || handler.jobs == nil {
		return queue.NewRetryableError(sharedrepository.ErrUnavailable)
	}
	if err := queue.ValidateHandlerJob(job, queue.KindRecomputeAIRun); err != nil {
		return queue.NewPermanentError(err)
	}
	args, err := DecodeAIRunRecomputeJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	run, err := handler.runs.FindRun(ctx, args.RunID)
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	if (run.Status != intelligencedomain.RunStatusFailed && run.Status != intelligencedomain.RunStatusCancelled) || run.OwningJobID == nil || *run.OwningJobID <= 0 {
		return queue.NewPermanentError(fmt.Errorf("%w: AI run cannot be recomputed", sharedrepository.ErrConflict))
	}
	if *run.OwningJobID == job.ID {
		return queue.NewPermanentError(fmt.Errorf("%w: recovery job cannot own its target run", sharedrepository.ErrConflict))
	}
	owner, err := handler.jobs.FindJobReference(ctx, *run.OwningJobID)
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	if owner.Kind == queue.KindRecomputeAIRun {
		return queue.NewPermanentError(fmt.Errorf("%w: nested AI recovery job", sharedrepository.ErrConflict))
	}
	activation, err := handler.jobs.ReactivateByID(ctx, *run.OwningJobID)
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	if activation.PreviousState == "running" {
		return queue.NewRetryableError(fmt.Errorf("owning job is still running"))
	}
	return nil
}
