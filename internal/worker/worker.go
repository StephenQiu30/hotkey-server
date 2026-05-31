package worker

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type Queue interface {
	Claim(context.Context) (queue.Job, error)
	Complete(context.Context, string) (queue.Job, error)
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
	if queue == nil {
		panic("worker requires queue")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
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
			completed, err := w.queue.Complete(ctx, job.ID)
			if err != nil {
				w.logger.Warn("worker complete failed", "job_id", job.ID, "job_type", job.Type, "error", err)
				continue
			}
			w.logger.Info("completed placeholder job", "job_id", completed.ID, "job_type", completed.Type)
		}
	}
}

func (w *Worker) Shutdown(context.Context) error {
	return nil
}
