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

func TestNewSendDailyEmailHandlerRejectsNilService(t *testing.T) {
	assertPanic(t, func() {
		NewSendDailyEmailHandler(nil)
	})
}

func TestSendWeeklyEmailHandlerPassesPayloadToMailService(t *testing.T) {
	payload, err := json.Marshal(queue.SendWeeklyEmailPayload{ReportID: "wr-1", RecipientUserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	service := &recordingWeeklyMailService{}
	handler := NewSendWeeklyEmailHandler(service)

	if err := handler.Handle(context.Background(), queue.Job{
		ID:      "job-1",
		Type:    queue.JobTypeSendWeeklyEmail,
		Payload: payload,
		Attempt: 1,
	}); err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	if service.input.ReportID != "wr-1" || service.input.RecipientUserID != "user-1" || service.input.Attempt != 1 {
		t.Fatalf("unexpected mail service input: %#v", service.input)
	}
}

func TestNewSendWeeklyEmailHandlerRejectsNilService(t *testing.T) {
	assertPanic(t, func() {
		NewSendWeeklyEmailHandler(nil)
	})
}

type recordingMailService struct {
	input servicemail.SendDailyEmailInput
}

func (s *recordingMailService) SendDailyEmail(_ context.Context, input servicemail.SendDailyEmailInput) (servicemail.Delivery, error) {
	s.input = input
	return servicemail.Delivery{Status: servicemail.DeliveryStatusSent}, nil
}

type recordingWeeklyMailService struct {
	input servicemail.SendWeeklyEmailInput
}

func (s *recordingWeeklyMailService) SendWeeklyEmail(_ context.Context, input servicemail.SendWeeklyEmailInput) (servicemail.Delivery, error) {
	s.input = input
	return servicemail.Delivery{Status: servicemail.DeliveryStatusSent}, nil
}
