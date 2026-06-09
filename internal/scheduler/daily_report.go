package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type DailyReportOptions struct {
	ReportAt string // 格式 "15:04"，默认 "06:00"
	Now      func() time.Time
}

type DailyReportScheduler struct {
	producer Producer
	reportAt string
	now      func() time.Time
}

func NewDailyReportScheduler(producer Producer, opts DailyReportOptions) *DailyReportScheduler {
	if producer == nil {
		panic("daily report scheduler requires producer")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	reportAt := opts.ReportAt
	if reportAt == "" {
		reportAt = "06:00"
	}
	return &DailyReportScheduler{
		producer: producer,
		reportAt: reportAt,
		now:      now,
	}
}

func (s *DailyReportScheduler) Tick(ctx context.Context) error {
	now := s.now()
	current := now.Format("15:04")
	if current != s.reportAt {
		return nil
	}

	date := now.Format("2006-01-02")
	payload, err := json.Marshal(queue.GenerateDailyReportPayload{Date: date})
	if err != nil {
		return err
	}
	_, err = s.producer.Enqueue(ctx, queue.EnqueueRequest{
		Type:           queue.JobTypeGenerateDailyReport,
		Payload:        payload,
		IdempotencyKey: fmt.Sprintf("generate_daily_report:%s", date),
	})
	return err
}
