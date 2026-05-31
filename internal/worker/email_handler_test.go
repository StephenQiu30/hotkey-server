package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicemail "github.com/StephenQiu30/hotkey-server/internal/service/mail"
)

func TestSendDailyEmailHandlerPassesPayloadToMailService(t *testing.T) {
	payload, err := json.Marshal(queue.SendDailyEmailPayload{ReportID: "report-1", RecipientUserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	service := &recordingMailService{}
	handler := NewSendDailyEmailHandler(service)

	if err := handler.Handle(context.Background(), queue.Job{
		ID:      "job-1",
		Type:    queue.JobTypeSendDailyEmail,
		Payload: payload,
		Attempt: 2,
	}); err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	if service.input.ReportID != "report-1" || service.input.RecipientUserID != "user-1" || service.input.Attempt != 2 {
		t.Fatalf("unexpected mail service input: %#v", service.input)
	}
}

type recordingMailService struct {
	input servicemail.SendDailyEmailInput
}

func (s *recordingMailService) SendDailyEmail(_ context.Context, input servicemail.SendDailyEmailInput) (servicemail.Delivery, error) {
	s.input = input
	return servicemail.Delivery{Status: servicemail.DeliveryStatusSent}, nil
}
