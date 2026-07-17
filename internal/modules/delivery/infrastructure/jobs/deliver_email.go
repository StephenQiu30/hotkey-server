package jobs

import (
	"context"
	"fmt"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

type MessageReader interface {
	Message(context.Context, int64) (deliveryapplication.MailMessage, int, error)
}

type DeliverEmailHandler struct {
	service *deliveryapplication.EmailService
	reader  MessageReader
}

func NewDeliverEmailHandler(service *deliveryapplication.EmailService, reader MessageReader) (*DeliverEmailHandler, error) {
	if service == nil || reader == nil {
		return nil, fmt.Errorf("deliver email dependencies are required")
	}
	return &DeliverEmailHandler{service: service, reader: reader}, nil
}

func (handler *DeliverEmailHandler) Handle(ctx context.Context, job queue.Job) error {
	if handler == nil || handler.service == nil || handler.reader == nil {
		return queue.NewPermanentError(fmt.Errorf("deliver email handler unavailable"))
	}
	if err := queue.ValidateHandlerJob(job, queue.KindDeliverEmail); err != nil {
		return queue.NewPermanentError(err)
	}
	message, attempt, err := handler.reader.Message(ctx, job.Payload.EntityID)
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	return queue.ClassifyHandlerError(ctx, handler.service.Deliver(ctx, job.Payload.EntityID, message, attempt))
}
