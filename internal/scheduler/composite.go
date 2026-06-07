package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Ticker interface {
	Tick(ctx context.Context) error
}

type CompositeScheduler struct {
	schedulers []Ticker
	interval   time.Duration
}

func NewCompositeScheduler(schedulers ...Ticker) *CompositeScheduler {
	return &CompositeScheduler{
		schedulers: schedulers,
		interval:   time.Minute,
	}
}

func (s *CompositeScheduler) Run(ctx context.Context) error {
	for _, sched := range s.schedulers {
		if err := sched.Tick(ctx); err != nil {
			slog.Warn("composite scheduler tick failed", "error", err)
		}
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for _, sched := range s.schedulers {
				if err := sched.Tick(ctx); err != nil {
					slog.Warn("composite scheduler tick failed", "error", err)
				}
			}
		}
	}
}

func (s *CompositeScheduler) Shutdown(context.Context) error {
	return nil
}
