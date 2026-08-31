package jobs

import (
	"context"
	"fmt"
	"time"

	alertapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type AlertEvaluator interface {
	Evaluate(context.Context, alertapplication.EventUpdateRef) (alertapplication.EvaluationResult, error)
}

// NewEvaluateJob builds the complete Alert job envelope. Summary fan-out is
// deliberately owned by the Event heat handler and is not a dependency here.
func NewEvaluateJob(ref alertapplication.EventUpdateRef, scheduledAt time.Time) (queue.Job, error) {
	if err := ref.Validate(); err != nil || scheduledAt.IsZero() {
		return queue.Job{}, fmt.Errorf("%w: invalid alert evaluation job", sharedrepository.ErrInvalidInput)
	}
	job := queue.Job{
		Kind:      queue.KindEvaluateEventAlerts,
		UniqueKey: queue.StableJobKey(queue.KindEvaluateEventAlerts, ref.ID, ref.Version, ref.EvidenceSetHash),
		Payload: queue.Payload{
			EntityID: ref.ID, EntityVersion: ref.Version, InputHash: ref.EvidenceSetHash,
		},
		ScheduledAt: scheduledAt.UTC(), MaxAttempts: 5, Priority: 3,
	}
	if err := job.Validate(); err != nil {
		return queue.Job{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	return job, nil
}

type EvaluateHandler struct{ evaluator AlertEvaluator }

func NewEvaluateHandler(evaluator AlertEvaluator) *EvaluateHandler {
	return &EvaluateHandler{evaluator: evaluator}
}

func (handler *EvaluateHandler) Handle(ctx context.Context, job queue.Job) error {
	if handler == nil || handler.evaluator == nil {
		return queue.NewPermanentError(fmt.Errorf("alert evaluator is unavailable"))
	}
	if err := queue.ValidateHandlerJob(job, queue.KindEvaluateEventAlerts); err != nil {
		return queue.NewPermanentError(err)
	}
	ref := alertapplication.EventUpdateRef{ID: job.Payload.EntityID, Version: job.Payload.EntityVersion, EvidenceSetHash: job.Payload.InputHash}
	if err := ref.Validate(); err != nil {
		return queue.NewPermanentError(err)
	}
	_, err := handler.evaluator.Evaluate(ctx, ref)
	return queue.ClassifyHandlerError(ctx, err)
}
