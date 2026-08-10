package jobs

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
)

const DefaultWebPushDispatchBatchSize = 50

type webPushDeliveryService interface {
	DispatchNext(context.Context) (application.DispatchWebPushDeliveryResult, error)
}

type WebPushDispatcher struct{ service webPushDeliveryService }

func NewWebPushDispatcher(service *application.WebPushDeliveryService) (*WebPushDispatcher, error) {
	return newWebPushDispatcher(service)
}

func newWebPushDispatcher(service webPushDeliveryService) (*WebPushDispatcher, error) {
	if service == nil {
		return nil, fmt.Errorf("Web Push delivery service is required")
	}
	return &WebPushDispatcher{service: service}, nil
}

func (dispatcher *WebPushDispatcher) DispatchBatch(ctx context.Context, limit int) (int, error) {
	if dispatcher == nil || dispatcher.service == nil || limit <= 0 || limit > 200 {
		return 0, fmt.Errorf("Web Push dispatch batch is invalid")
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
