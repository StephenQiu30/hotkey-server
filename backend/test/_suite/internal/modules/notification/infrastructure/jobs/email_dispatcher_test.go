package jobs

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
)

type emailDeliveryServiceStub struct {
	remaining int
	calls     int
}

func (stub *emailDeliveryServiceStub) DispatchNext(context.Context) (application.DispatchEmailDeliveryResult, error) {
	stub.calls++
	if stub.remaining == 0 {
		return application.DispatchEmailDeliveryResult{}, nil
	}
	stub.remaining--
	return application.DispatchEmailDeliveryResult{Claimed: true, UserNotificationID: int64(stub.calls), Status: "succeeded"}, nil
}

func TestEmailDispatcherStopsAtEmptyOutbox(t *testing.T) {
	service := &emailDeliveryServiceStub{remaining: 2}
	dispatcher, err := newEmailDispatcher(service)
	if err != nil {
		t.Fatal(err)
	}
	count, err := dispatcher.DispatchBatch(context.Background(), 20)
	if err != nil || count != 2 || service.calls != 3 {
		t.Fatalf("DispatchBatch() = %d/%v calls=%d", count, err, service.calls)
	}
}
