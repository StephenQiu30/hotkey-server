package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type HotspotSchedulerOptions struct {
	Now      func() time.Time
	Interval time.Duration
	Window   time.Duration
}

type HotspotScheduler struct {
	producer Producer
	now      func() time.Time
	interval time.Duration
	window   time.Duration
}

func NewHotspotScheduler(producer Producer, opts HotspotSchedulerOptions) *HotspotScheduler {
	if producer == nil {
		panic("hotspot scheduler requires producer")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	window := opts.Window
	if window <= 0 {
		window = 24 * time.Hour
	}
	return &HotspotScheduler{
		producer: producer,
		now:      now,
		interval: interval,
		window:   window,
	}
}

func (s *HotspotScheduler) Tick(ctx context.Context) error {
	now := s.now().UTC()
	truncated := now.Truncate(time.Hour)
	windowStart := now.Add(-s.window)

	clusterPayload, err := json.Marshal(queue.ClusterHotspotsPayload{
		WindowStart: windowStart,
		WindowEnd:   now,
	})
	if err != nil {
		return err
	}
	clusterKey := fmt.Sprintf("cluster_hotspots:%s", truncated.Format("2006-01-02T15"))
	_, err = s.producer.Enqueue(ctx, queue.EnqueueRequest{
		Type:           queue.JobTypeClusterHotspots,
		Payload:        clusterPayload,
		IdempotencyKey: clusterKey,
	})
	if err != nil {
		return err
	}

	scorePayload, err := json.Marshal(queue.ScoreHotspotsPayload{
		ClusterRunID: clusterKey,
	})
	if err != nil {
		return err
	}
	scoreKey := fmt.Sprintf("score_hotspots:%s", truncated.Format("2006-01-02T15"))
	_, err = s.producer.Enqueue(ctx, queue.EnqueueRequest{
		Type:           queue.JobTypeScoreHotspots,
		Payload:        scorePayload,
		IdempotencyKey: scoreKey,
	})
	return err
}

func (s *HotspotScheduler) Run(ctx context.Context) error {
	if err := s.Tick(ctx); err != nil {
		slog.Warn("hotspot scheduler tick failed", "error", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				slog.Warn("hotspot scheduler tick failed", "error", err)
			}
		}
	}
}

func (s *HotspotScheduler) Shutdown(context.Context) error {
	return nil
}
