package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type Queue interface {
	Claim(context.Context) (queue.Job, error)
}

type RedisHealth interface {
	Ping(context.Context) error
}

type Worker struct {
	queue  Queue
	redis  RedisHealth
	logger *slog.Logger
}

func New(queue Queue, redis RedisHealth, logger *slog.Logger) *Worker {
	return &Worker{queue: queue, redis: redis, logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	if w.redis != nil {
		if err := w.redis.Ping(ctx); err != nil {
			w.logger.Warn("redis unavailable for worker", "error", err)
		}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			job, err := w.queue.Claim(ctx)
			if errors.Is(err, queue.ErrNoJobs) {
				continue
			}
			if err != nil {
				w.logger.Warn("worker claim failed", "error", err)
				continue
			}
			w.logger.Info("claimed job without business handler", "job_id", job.ID, "job_type", job.Type)
		}
	}
}

func (w *Worker) Shutdown(context.Context) error {
	return nil
}
