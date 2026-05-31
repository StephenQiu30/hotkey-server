package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type Producer interface {
	Enqueue(context.Context, queue.EnqueueRequest) (queue.Job, error)
}

type HourlyCollectOptions struct {
	SourceID string
	Now      func() time.Time
	Interval time.Duration
}

type HourlyCollectScheduler struct {
	producer Producer
	sourceID string
	now      func() time.Time
	interval time.Duration
}

func NewHourlyCollectScheduler(producer Producer, opts HourlyCollectOptions) *HourlyCollectScheduler {
	if producer == nil {
		panic("hourly collect scheduler requires producer")
	}
	if opts.SourceID == "" {
		panic("hourly collect scheduler requires source id")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	return &HourlyCollectScheduler{
		producer: producer,
		sourceID: opts.SourceID,
		now:      now,
		interval: interval,
	}
}

func (s *HourlyCollectScheduler) Tick(ctx context.Context) error {
	scheduledFor := s.now().UTC().Truncate(time.Hour)
	payload, err := json.Marshal(queue.CollectSourcePayload{
		SourceID:     s.sourceID,
		ScheduledFor: scheduledFor,
	})
	if err != nil {
		return err
	}
	_, err = s.producer.Enqueue(ctx, queue.EnqueueRequest{
		Type:           queue.JobTypeCollectSource,
		Payload:        payload,
		IdempotencyKey: fmt.Sprintf("collect_source:%s:%s", s.sourceID, scheduledFor.Format("2006-01-02T15")),
	})
	return err
}

func (s *HourlyCollectScheduler) Run(ctx context.Context) error {
	if err := s.Tick(ctx); err != nil {
		slog.Warn("hourly collect scheduler tick failed", "error", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				slog.Warn("hourly collect scheduler tick failed", "error", err)
			}
		}
	}
}

func (s *HourlyCollectScheduler) Shutdown(context.Context) error {
	return nil
}
