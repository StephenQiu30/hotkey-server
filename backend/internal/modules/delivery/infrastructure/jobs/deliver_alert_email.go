package jobs

import (
	"context"
	"fmt"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type DeliverAlertEmailHandler struct {
	service *deliveryapplication.AlertEmailService
}

func NewDeliverAlertEmailHandler(service *deliveryapplication.AlertEmailService) (*DeliverAlertEmailHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("deliver alert email service is required")
	}
	return &DeliverAlertEmailHandler{service: service}, nil
}

func (handler *DeliverAlertEmailHandler) Handle(ctx context.Context, job queue.Job) error {
	if handler == nil || handler.service == nil {
		return queue.NewPermanentError(fmt.Errorf("deliver alert email handler unavailable"))
	}
	if err := queue.ValidateHandlerJob(job, queue.KindDeliverAlertEmail); err != nil {
		return queue.NewPermanentError(err)
	}
	return queue.ClassifyHandlerError(ctx, handler.service.Deliver(ctx, job.Payload.EntityID))
}
