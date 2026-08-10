package jobs

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
)

type webPushDeliveryServiceStub struct {
	remaining int
	calls     int
}

func (stub *webPushDeliveryServiceStub) DispatchNext(context.Context) (application.DispatchWebPushDeliveryResult, error) {
	stub.calls++
	if stub.remaining == 0 {
		return application.DispatchWebPushDeliveryResult{}, nil
	}
	stub.remaining--
	return application.DispatchWebPushDeliveryResult{Claimed: true, UserNotificationID: int64(stub.calls), Status: "succeeded"}, nil
}

func TestWebPushDispatcherStopsAtEmptyOutbox(t *testing.T) {
	service := &webPushDeliveryServiceStub{remaining: 2}
	dispatcher, err := newWebPushDispatcher(service)
	if err != nil {
		t.Fatal(err)
	}
	count, err := dispatcher.DispatchBatch(context.Background(), 20)
	if err != nil || count != 2 || service.calls != 3 {
		t.Fatalf("DispatchBatch() = %d/%v calls=%d", count, err, service.calls)
	}
}
