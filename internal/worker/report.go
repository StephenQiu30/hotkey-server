package worker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

type ReportService interface {
	GenerateChannelReport(context.Context, servicereport.GenerateReportInput) (servicereport.DailyReport, error)
}

type GenerateReportHandler struct {
	service ReportService
}

func NewGenerateReportHandler(service ReportService) *GenerateReportHandler {
	return &GenerateReportHandler{service: service}
}

func (h *GenerateReportHandler) Handle(ctx context.Context, job queue.Job) error {
	if h.service == nil {
		return errors.New("generate report handler missing service")
	}
	var payload queue.GenerateDailyReportPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	if _, err := time.Parse("2006-01-02", payload.Date); err != nil {
		return errors.New("generate_daily_report payload requires date in YYYY-MM-DD format")
	}
	_, err := h.service.GenerateChannelReport(ctx, servicereport.GenerateReportInput{Date: payload.Date, ChannelID: "default"})
	if errors.Is(err, servicereport.ErrFailedConfig) {
		return nil
	}
	return err
}
