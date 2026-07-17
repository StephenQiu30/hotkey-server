package jobs

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

type Builder func(context.Context, int64) error

type BuildHandler struct{ builder Builder }

func NewBuildHandler(builder Builder) (*BuildHandler, error) {
	if builder == nil {
		return nil, fmt.Errorf("report builder is required")
	}
	return &BuildHandler{builder: builder}, nil
}

func (handler *BuildHandler) Handle(ctx context.Context, job queue.Job) error {
	if handler == nil || handler.builder == nil {
		return queue.NewPermanentError(fmt.Errorf("report builder is unavailable"))
	}
	if err := queue.ValidateHandlerJob(job, queue.KindBuildReport); err != nil {
		return queue.NewPermanentError(err)
	}
	return queue.ClassifyHandlerError(ctx, handler.builder(ctx, job.Payload.EntityID))
}
