// Package jobs contains queue adapters for Knowledge work. The handlers pass
// only a document/proposal ID across River; all Vault and database facts are
// reread through application ports.
package jobs

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

type IDRunner func(context.Context, int64) error

type Handler struct {
	kind   string
	runner IDRunner
}

func NewHandler(kind string, runner IDRunner) (*Handler, error) {
	if kind != queue.KindProjectKnowledge && kind != queue.KindReconcileKnowledge {
		return nil, fmt.Errorf("invalid knowledge job kind")
	}
	if runner == nil {
		return nil, fmt.Errorf("knowledge job runner is required")
	}
	return &Handler{kind: kind, runner: runner}, nil
}

func (handler *Handler) Handle(ctx context.Context, job queue.Job) error {
	if handler == nil || handler.runner == nil {
		return queue.NewPermanentError(fmt.Errorf("knowledge job handler is unavailable"))
	}
	if err := queue.ValidateHandlerJob(job, handler.kind); err != nil {
		return queue.NewPermanentError(err)
	}
	return queue.ClassifyHandlerError(ctx, handler.runner(ctx, job.Payload.EntityID))
}
