package jobs

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
)

const DefaultEmailDispatchBatchSize = 20

type emailDeliveryService interface {
	DispatchNext(context.Context) (application.DispatchEmailDeliveryResult, error)
}

type EmailDispatcher struct {
	service emailDeliveryService
}

func NewEmailDispatcher(service *application.EmailDeliveryService) (*EmailDispatcher, error) {
	return newEmailDispatcher(service)
}

func newEmailDispatcher(service emailDeliveryService) (*EmailDispatcher, error) {
	if service == nil {
		return nil, fmt.Errorf("notification email delivery service is required")
	}
	return &EmailDispatcher{service: service}, nil
}

func (dispatcher *EmailDispatcher) DispatchBatch(ctx context.Context, limit int) (int, error) {
	if dispatcher == nil || dispatcher.service == nil || limit <= 0 || limit > 100 {
		return 0, fmt.Errorf("notification email dispatch batch is invalid")
	}
	dispatched := 0
	for dispatched < limit {
		result, err := dispatcher.service.DispatchNext(ctx)
		if err != nil {
			return dispatched, err
		}
		if !result.Claimed {
			return dispatched, nil
		}
		dispatched++
	}
	return dispatched, nil
}
