package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type DailyEmailRecipient struct {
	UserID       string
	EmailEnabled bool
	DailySendAt  string
}

type DailyEmailOptions struct {
	ReportID           string
	DefaultDailySendAt string
	Recipients         []DailyEmailRecipient
	Now                func() time.Time
}

type DailyEmailScheduler struct {
	producer           Producer
	reportID           string
	defaultDailySendAt string
	recipients         []DailyEmailRecipient
	now                func() time.Time
}

func NewDailyEmailScheduler(producer Producer, opts DailyEmailOptions) *DailyEmailScheduler {
	if producer == nil {
		panic("daily email scheduler requires producer")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &DailyEmailScheduler{
		producer:           producer,
		reportID:           opts.ReportID,
		defaultDailySendAt: opts.DefaultDailySendAt,
		recipients:         opts.Recipients,
		now:                now,
	}
}

func (s *DailyEmailScheduler) Tick(ctx context.Context) error {
	now := s.now()
	current := now.Format("15:04")
	today := now.Format("2006-01-02")
	for _, recipient := range s.recipients {
		if !recipient.EmailEnabled {
			continue
		}
		sendAt := recipient.DailySendAt
		if sendAt == "" {
			sendAt = s.defaultDailySendAt
		}
		if sendAt != current {
			continue
		}
		payload, err := json.Marshal(queue.SendDailyEmailPayload{
			ReportID:        s.reportID,
			RecipientUserID: recipient.UserID,
		})
		if err != nil {
			return err
		}
		if _, err := s.producer.Enqueue(ctx, queue.EnqueueRequest{
			Type:           queue.JobTypeSendDailyEmail,
			Payload:        payload,
			IdempotencyKey: fmt.Sprintf("send_daily_email:%s:%s:%s", s.reportID, recipient.UserID, today),
		}); err != nil {
			return err
		}
	}
	return nil
}
