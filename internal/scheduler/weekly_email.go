package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type WeeklyEmailRecipient struct {
	UserID        string
	EmailEnabled  bool
	WeeklySendAt  string
}

type WeeklyEmailOptions struct {
	ReportID           string
	WeeklySendDay      time.Weekday
	WeeklySendAt       string
	DefaultWeeklySendAt string
	Recipients         []WeeklyEmailRecipient
	Now                func() time.Time
}

type WeeklyEmailScheduler struct {
	producer            Producer
	reportID            string
	weeklySendDay       time.Weekday
	weeklySendAt        string
	defaultWeeklySendAt string
	recipients          []WeeklyEmailRecipient
	now                 func() time.Time
}

func NewWeeklyEmailScheduler(producer Producer, opts WeeklyEmailOptions) *WeeklyEmailScheduler {
	if producer == nil {
		panic("weekly email scheduler requires producer")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	sendAt := opts.WeeklySendAt
	if sendAt == "" {
		sendAt = opts.DefaultWeeklySendAt
	}
	if sendAt == "" {
		sendAt = "09:00"
	}
	return &WeeklyEmailScheduler{
		producer:            producer,
		reportID:            opts.ReportID,
		weeklySendDay:       opts.WeeklySendDay,
		weeklySendAt:        sendAt,
		defaultWeeklySendAt: opts.DefaultWeeklySendAt,
		recipients:          opts.Recipients,
		now:                 now,
	}
}

func (s *WeeklyEmailScheduler) Tick(ctx context.Context) error {
	now := s.now()
	if now.Weekday() != s.weeklySendDay {
		return nil
	}
	current := now.Format("15:04")
	if current != s.weeklySendAt {
		return nil
	}
	weekOf := isoWeek(now)
	for _, recipient := range s.recipients {
		if !recipient.EmailEnabled {
			continue
		}
		payload, err := json.Marshal(queue.SendWeeklyEmailPayload{
			ReportID:        s.reportID,
			RecipientUserID: recipient.UserID,
		})
		if err != nil {
			return err
		}
		if _, err := s.producer.Enqueue(ctx, queue.EnqueueRequest{
			Type:           queue.JobTypeSendWeeklyEmail,
			Payload:        payload,
			IdempotencyKey: fmt.Sprintf("send_weekly_email:%s:%s:%s", s.reportID, recipient.UserID, weekOf),
		}); err != nil {
			return err
		}
	}
	return nil
}

func isoWeek(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}
