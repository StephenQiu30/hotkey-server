package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

type ReportService interface {
	GenerateChannelReport(context.Context, servicereport.GenerateReportInput) (servicereport.DailyReport, error)
}

// QueueProducer 抽象队列入队能力，供 handler 链式触发下游任务。
type QueueProducer interface {
	Enqueue(context.Context, queue.EnqueueRequest) (queue.Job, error)
}

type GenerateReportHandler struct {
	service    ReportService
	producer   QueueProducer
	recipients []string
}

func NewGenerateReportHandler(service ReportService) *GenerateReportHandler {
	return &GenerateReportHandler{service: service}
}

// NewGenerateReportHandlerWithMail 创建带邮件链式触发的日报 handler。
// 日报生成成功后，自动为每个 recipient 入队 send_daily_email 任务。
func NewGenerateReportHandlerWithMail(service ReportService, producer QueueProducer, recipients []string) *GenerateReportHandler {
	return &GenerateReportHandler{
		service:    service,
		producer:   producer,
		recipients: recipients,
	}
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
	report, err := h.service.GenerateChannelReport(ctx, servicereport.GenerateReportInput{Date: payload.Date, ChannelID: "default"})
	if errors.Is(err, servicereport.ErrFailedConfig) {
		return nil
	}
	if err != nil {
		return err
	}

	// 日报生成成功，链式入队邮件任务
	return h.enqueueEmails(ctx, report.ID, payload.Date)
}

func (h *GenerateReportHandler) enqueueEmails(ctx context.Context, reportID string, date string) error {
	if h.producer == nil || len(h.recipients) == 0 {
		return nil
	}
	for _, userID := range h.recipients {
		emailPayload, err := json.Marshal(queue.SendDailyEmailPayload{
			ReportID:        reportID,
			RecipientUserID: userID,
		})
		if err != nil {
			return fmt.Errorf("marshal send_daily_email payload: %w", err)
		}
		if _, err := h.producer.Enqueue(ctx, queue.EnqueueRequest{
			Type:           queue.JobTypeSendDailyEmail,
			Payload:        emailPayload,
			IdempotencyKey: fmt.Sprintf("send_daily_email:%s:%s:%s", reportID, userID, date),
		}); err != nil {
			return fmt.Errorf("enqueue send_daily_email for %s: %w", userID, err)
		}
	}
	return nil
}
