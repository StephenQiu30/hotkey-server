package jobs

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

type SummaryGenerator func(context.Context, int64) error

// SummaryHandler is the final P0 boundary. It passes only the Event ID to the
// existing Event-owned evidence snapshot service; provider and prompt details
// stay behind Intelligence Application.
type SummaryHandler struct {
	service SummaryGenerator
}

func NewSummaryHandler(service SummaryGenerator) (*SummaryHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("summary handler dependencies are required")
	}
	return &SummaryHandler{service: service}, nil
}

func (handler *SummaryHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindGenerateEventSummary); err != nil {
		return queue.NewPermanentError(err)
	}
	if err := handler.service(ctx, job.Payload.EntityID); err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	return nil
}
