package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

func TestGenerateReportHandler(t *testing.T) {
	body, _ := json.Marshal(queue.GenerateDailyReportPayload{Date: "2026-05-31"})
	service := &mockReportService{}
	handler := NewGenerateReportHandler(service)
	if err := handler.Handle(context.Background(), queue.Job{Payload: body}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if service.input.Date != "2026-05-31" || service.input.ChannelID != "default" {
		t.Fatalf("unexpected input: %+v", service.input)
	}
}

func TestGenerateReportHandlerTreatsFailedConfigAsNonFatal(t *testing.T) {
	body, _ := json.Marshal(queue.GenerateDailyReportPayload{Date: "2026-05-31"})
	handler := NewGenerateReportHandler(&mockReportService{err: servicereport.ErrFailedConfig})
	if err := handler.Handle(context.Background(), queue.Job{Payload: body}); err != nil {
		t.Fatalf("expected failed_config to be non-fatal, got %v", err)
	}
}

func TestGenerateReportHandlerNilService(t *testing.T) {
	body, _ := json.Marshal(queue.GenerateDailyReportPayload{Date: "2026-05-31"})
	if err := NewGenerateReportHandler(nil).Handle(context.Background(), queue.Job{Payload: body}); err == nil {
		t.Fatalf("expected nil service error")
	}
}

type mockReportService struct {
	input servicereport.GenerateReportInput
	err   error
}

func (m *mockReportService) GenerateChannelReport(_ context.Context, input servicereport.GenerateReportInput) (servicereport.DailyReport, error) {
	m.input = input
	if m.err != nil {
		return servicereport.DailyReport{}, m.err
	}
	return servicereport.DailyReport{ID: "rpt-1"}, nil
}
